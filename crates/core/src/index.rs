//! The rebuildable cache.
//!
//! Nothing in here is authoritative. Delete `.brain/index.db`, run `brain
//! index`, and you are back to identical state derived from the markdown.

use crate::note::{Note, slug_for};
use crate::provider::Provider;
use anyhow::{Context, Result};
use rusqlite::{Connection, params};
use std::collections::HashSet;
use std::path::{Path, PathBuf};
use walkdir::WalkDir;

const SCHEMA: &str = r#"
CREATE TABLE IF NOT EXISTS notes (
    slug  TEXT PRIMARY KEY,
    path  TEXT NOT NULL,
    title TEXT NOT NULL,
    kind  TEXT NOT NULL,
    body  TEXT NOT NULL,
    hash  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS aliases (
    slug  TEXT NOT NULL REFERENCES notes(slug) ON DELETE CASCADE,
    alias TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS edges (
    src_slug TEXT NOT NULL REFERENCES notes(slug) ON DELETE CASCADE,
    pred     TEXT NOT NULL,
    obj      TEXT NOT NULL,
    conf     REAL NOT NULL,
    src      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS edges_src ON edges(src_slug);
CREATE INDEX IF NOT EXISTS edges_obj ON edges(obj);
CREATE TABLE IF NOT EXISTS embeddings (
    slug TEXT PRIMARY KEY REFERENCES notes(slug) ON DELETE CASCADE,
    dim  INTEGER NOT NULL,
    vec  BLOB NOT NULL
);
"#;

pub struct Index {
    pub vault: PathBuf,
    pub conn: Connection,
}

#[derive(Debug, Default)]
pub struct SyncReport {
    pub added: usize,
    pub updated: usize,
    pub removed: usize,
    pub unchanged: usize,
}

impl Index {
    pub fn open(vault: &Path) -> Result<Index> {
        let dir = vault.join(".brain");
        std::fs::create_dir_all(&dir).with_context(|| format!("creating {}", dir.display()))?;

        let conn = Connection::open(dir.join("index.db"))?;
        conn.pragma_update(None, "journal_mode", "WAL")?;
        conn.pragma_update(None, "foreign_keys", "ON")?;
        conn.execute_batch(SCHEMA)?;

        Ok(Index { vault: vault.to_path_buf(), conn })
    }

    /// Read every note in the vault and reconcile it against the cache.
    /// Only notes whose content hash changed get re-parsed into edges and
    /// marked for re-embedding.
    pub fn sync(&mut self) -> Result<SyncReport> {
        let mut report = SyncReport::default();
        let mut seen: HashSet<String> = HashSet::new();

        let files: Vec<PathBuf> = WalkDir::new(&self.vault)
            .into_iter()
            .filter_entry(|e| e.file_name() != ".brain" && !e.file_name().to_string_lossy().starts_with('.'))
            .filter_map(|e| e.ok())
            .filter(|e| e.file_type().is_file() && e.path().extension().is_some_and(|x| x == "md"))
            .map(|e| e.into_path())
            .collect();

        let tx = self.conn.transaction()?;

        for path in files {
            let slug = slug_for(&self.vault, &path);
            seen.insert(slug.clone());

            let raw = std::fs::read_to_string(&path).with_context(|| format!("reading {}", path.display()))?;
            let note = Note::parse(&self.vault, &path, &raw)?;
            let hash = note.hash.to_string();

            let existing: Option<String> = tx
                .query_row("SELECT hash FROM notes WHERE slug = ?1", params![slug], |r| r.get(0))
                .ok();

            match existing {
                Some(h) if h == hash => {
                    report.unchanged += 1;
                    continue;
                }
                Some(_) => report.updated += 1,
                None => report.added += 1,
            }

            tx.execute(
                "INSERT INTO notes (slug, path, title, kind, body, hash) VALUES (?1,?2,?3,?4,?5,?6)
                 ON CONFLICT(slug) DO UPDATE SET path=?2, title=?3, kind=?4, body=?5, hash=?6",
                params![slug, path.to_string_lossy(), note.title, note.kind, note.body, hash],
            )?;

            // Content changed, so the old derived rows are stale by definition.
            tx.execute("DELETE FROM edges WHERE src_slug = ?1", params![slug])?;
            tx.execute("DELETE FROM aliases WHERE slug = ?1", params![slug])?;
            tx.execute("DELETE FROM embeddings WHERE slug = ?1", params![slug])?;

            for a in &note.aliases {
                tx.execute("INSERT INTO aliases (slug, alias) VALUES (?1,?2)", params![slug, a])?;
            }
            for e in &note.edges {
                tx.execute(
                    "INSERT INTO edges (src_slug, pred, obj, conf, src) VALUES (?1,?2,?3,?4,?5)",
                    params![slug, e.pred, e.obj, e.conf, e.src.as_str()],
                )?;
            }
        }

        // Notes deleted from disk must disappear from the cache too, or
        // retrieval starts citing files that no longer exist.
        let stale: Vec<String> = {
            let mut stmt = tx.prepare("SELECT slug FROM notes")?;
            let all: Vec<String> = stmt.query_map([], |r| r.get(0))?.collect::<Result<_, _>>()?;
            all.into_iter().filter(|s| !seen.contains(s)).collect()
        };
        for slug in &stale {
            tx.execute("DELETE FROM notes WHERE slug = ?1", params![slug])?;
            report.removed += 1;
        }

        tx.commit()?;
        Ok(report)
    }

    pub fn pending_embeddings(&self) -> Result<Vec<(String, String, String, String)>> {
        let mut stmt = self.conn.prepare(
            "SELECT n.slug, n.title, n.kind, n.body FROM notes n
             LEFT JOIN embeddings e ON e.slug = n.slug WHERE e.slug IS NULL",
        )?;
        let rows = stmt
            .query_map([], |r| Ok((r.get(0)?, r.get(1)?, r.get(2)?, r.get(3)?)))?
            .collect::<Result<Vec<_>, _>>()?;
        Ok(rows)
    }

    /// Embed everything that lacks a vector. Batched, because a round trip per
    /// note makes a 2000-note vault take minutes instead of seconds.
    pub fn embed_pending(&self, provider: &Provider, model: &str, batch: usize) -> Result<usize> {
        let pending = self.pending_embeddings()?;
        let mut done = 0;

        for chunk in pending.chunks(batch.max(1)) {
            let inputs: Vec<String> = chunk
                .iter()
                .map(|(_, title, kind, body)| format!("{title} ({kind})\n\n{body}"))
                .collect();

            let vectors = provider.embed(model, &inputs)?;
            for ((slug, ..), vec) in chunk.iter().zip(vectors) {
                self.conn.execute(
                    "INSERT INTO embeddings (slug, dim, vec) VALUES (?1,?2,?3)
                     ON CONFLICT(slug) DO UPDATE SET dim=?2, vec=?3",
                    params![slug, vec.len() as i64, f32_to_blob(&vec)],
                )?;
                done += 1;
            }
        }
        Ok(done)
    }

    pub fn note_count(&self) -> Result<i64> {
        Ok(self.conn.query_row("SELECT COUNT(*) FROM notes", [], |r| r.get(0))?)
    }

    pub fn edge_count(&self) -> Result<i64> {
        Ok(self.conn.query_row("SELECT COUNT(*) FROM edges", [], |r| r.get(0))?)
    }
}

pub fn f32_to_blob(v: &[f32]) -> Vec<u8> {
    v.iter().flat_map(|x| x.to_le_bytes()).collect()
}

pub fn blob_to_f32(b: &[u8]) -> Vec<f32> {
    b.chunks_exact(4)
        .map(|c| f32::from_le_bytes([c[0], c[1], c[2], c[3]]))
        .collect()
}
