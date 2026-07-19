//! Parsing a vault note into the shape the index cares about.
//!
//! The markdown file is the source of truth; this module is a lossless-enough
//! reader, never a writer. Anything we cannot parse is preserved by simply not
//! touching the file.

use anyhow::Result;
use regex::Regex;
use serde::Deserialize;
use std::path::{Path, PathBuf};
use std::sync::OnceLock;

/// Where an edge came from, which is what determines how much we trust it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EdgeSrc {
    /// You wrote it — a `[[wikilink]]` or an explicit frontmatter relation.
    Stated,
    /// A model proposed it and you accepted it.
    Inferred,
    /// Pulled in from an external system.
    Imported,
}

impl EdgeSrc {
    fn parse(s: &str) -> Self {
        match s {
            "inferred" => Self::Inferred,
            "imported" => Self::Imported,
            _ => Self::Stated,
        }
    }

    pub fn as_str(self) -> &'static str {
        match self {
            Self::Stated => "stated",
            Self::Inferred => "inferred",
            Self::Imported => "imported",
        }
    }
}

#[derive(Debug, Clone)]
pub struct Edge {
    /// Predicate, e.g. `works_on`. Body wikilinks use `mentions`.
    pub pred: String,
    /// Target note slug, with `[[ ]]` stripped.
    pub obj: String,
    pub conf: f32,
    pub src: EdgeSrc,
}

#[derive(Debug, Clone)]
pub struct Note {
    pub slug: String,
    pub path: PathBuf,
    pub title: String,
    pub kind: String,
    pub aliases: Vec<String>,
    pub body: String,
    pub edges: Vec<Edge>,
    /// Content hash, so reindexing only re-embeds what actually changed.
    pub hash: u64,
}

#[derive(Debug, Default, Deserialize)]
struct Frontmatter {
    #[serde(default)]
    r#type: Option<String>,
    #[serde(default)]
    title: Option<String>,
    #[serde(default)]
    aliases: Vec<String>,
    #[serde(default)]
    relations: Vec<RawRelation>,
}

#[derive(Debug, Deserialize)]
struct RawRelation {
    pred: String,
    obj: String,
    #[serde(default = "one")]
    conf: f32,
    #[serde(default)]
    src: Option<String>,
}

fn one() -> f32 {
    1.0
}

fn wikilink_re() -> &'static Regex {
    static RE: OnceLock<Regex> = OnceLock::new();
    // [[target]] or [[target|display]]
    RE.get_or_init(|| Regex::new(r"\[\[([^\]\|#]+)(?:[#\|][^\]]*)?\]\]").unwrap())
}

/// `vault/people/Sameer Rao.md` -> `people/sameer-rao`
pub fn slug_for(vault: &Path, path: &Path) -> String {
    let rel = path.strip_prefix(vault).unwrap_or(path).with_extension("");
    rel.to_string_lossy()
        .to_lowercase()
        .replace(['_', ' '], "-")
        .replace('\\', "/")
}

/// Wikilinks may be written bare (`[[sameer-rao]]`) or as a path. We normalise
/// to the trailing segment and resolve against known slugs at query time —
/// full entity resolution lands in step 3.
pub fn normalize_link(target: &str) -> String {
    target
        .trim()
        .to_lowercase()
        .replace(['_', ' '], "-")
        .rsplit('/')
        .next()
        .unwrap_or("")
        .to_string()
}

fn hash(s: &str) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut h = std::collections::hash_map::DefaultHasher::new();
    s.hash(&mut h);
    h.finish()
}

/// Split `---\nyaml\n---\nbody` into its two halves.
fn split_frontmatter(raw: &str) -> (Option<&str>, &str) {
    let Some(rest) = raw.strip_prefix("---\n") else {
        return (None, raw);
    };
    match rest.find("\n---") {
        Some(end) => {
            let body = rest[end + 4..].trim_start_matches('\n');
            (Some(&rest[..end]), body)
        }
        None => (None, raw),
    }
}

impl Note {
    pub fn parse(vault: &Path, path: &Path, raw: &str) -> Result<Note> {
        let (fm_str, body) = split_frontmatter(raw);

        // A malformed frontmatter block should degrade to "no metadata", not
        // drop the note out of the index entirely.
        let fm: Frontmatter = fm_str
            .and_then(|s| serde_yaml_ng::from_str(s).ok())
            .unwrap_or_default();

        let slug = slug_for(vault, path);

        let mut edges: Vec<Edge> = fm
            .relations
            .into_iter()
            .map(|r| Edge {
                pred: r.pred,
                obj: normalize_link(r.obj.trim_matches(|c| c == '[' || c == ']')),
                conf: r.conf.clamp(0.0, 1.0),
                src: r.src.as_deref().map(EdgeSrc::parse).unwrap_or(EdgeSrc::Stated),
            })
            .collect();

        // Body wikilinks are things you typed, so they are stated at full
        // confidence with an untyped predicate.
        for cap in wikilink_re().captures_iter(body) {
            let obj = normalize_link(&cap[1]);
            if obj.is_empty() || edges.iter().any(|e| e.obj == obj) {
                continue;
            }
            edges.push(Edge {
                pred: "mentions".into(),
                obj,
                conf: 1.0,
                src: EdgeSrc::Stated,
            });
        }

        let title = fm.title.unwrap_or_else(|| {
            path.file_stem()
                .map(|s| s.to_string_lossy().into_owned())
                .unwrap_or_else(|| slug.clone())
        });

        Ok(Note {
            slug,
            path: path.to_path_buf(),
            title,
            kind: fm.r#type.unwrap_or_else(|| "note".into()),
            aliases: fm.aliases,
            hash: hash(raw),
            body: body.to_string(),
            edges,
        })
    }

    /// What actually gets embedded. Title and kind are prepended so a query
    /// like "who is sameer" can match a sparse stub note.
    pub fn embed_text(&self) -> String {
        format!("{} ({})\n\n{}", self.title, self.kind, self.body)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_frontmatter_relations_and_body_links() {
        let raw = "---\ntype: person\naliases: [Sam]\nrelations:\n  - { pred: works_on, obj: \"[[brain]]\", conf: 0.8, src: inferred }\n---\nWorks with [[Ana Diaz]] on [[brain]].\n";
        let n = Note::parse(Path::new("/v"), Path::new("/v/people/sameer.md"), raw).unwrap();

        assert_eq!(n.slug, "people/sameer");
        assert_eq!(n.kind, "person");
        assert_eq!(n.aliases, ["Sam"]);

        // [[brain]] already exists as a typed relation, so the body mention
        // must not create a duplicate weaker edge.
        assert_eq!(n.edges.len(), 2);
        let brain = n.edges.iter().find(|e| e.obj == "brain").unwrap();
        assert_eq!(brain.pred, "works_on");
        assert_eq!(brain.src, EdgeSrc::Inferred);

        let ana = n.edges.iter().find(|e| e.obj == "ana-diaz").unwrap();
        assert_eq!(ana.pred, "mentions");
        assert_eq!(ana.conf, 1.0);
    }

    #[test]
    fn malformed_frontmatter_still_indexes_body() {
        let raw = "---\nthis: is: not: yaml\n---\nhello [[world]]\n";
        let n = Note::parse(Path::new("/v"), Path::new("/v/a.md"), raw).unwrap();
        assert_eq!(n.kind, "note");
        assert_eq!(n.edges.len(), 1);
    }

    #[test]
    fn note_without_frontmatter_is_fine() {
        let n = Note::parse(Path::new("/v"), Path::new("/v/a.md"), "just text").unwrap();
        assert_eq!(n.body, "just text");
        assert!(n.edges.is_empty());
    }

    #[test]
    fn strips_wikilink_aliases_and_headings() {
        let n = Note::parse(Path::new("/v"), Path::new("/v/a.md"), "[[brain|the app]] [[brain#setup]]").unwrap();
        assert_eq!(n.edges.len(), 1);
        assert_eq!(n.edges[0].obj, "brain");
    }
}
