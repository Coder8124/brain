//! Retrieval: vector search, then one hop along stated edges.
//!
//! Brute-force cosine is deliberate. At vault scale (a few thousand notes) it
//! is well under 5ms, and it keeps the index a plain SQLite file with no
//! extension to install. Revisit if this vault ever passes ~50k notes.

use crate::index::{Index, blob_to_f32};
use crate::provider::Provider;
use anyhow::Result;
use rusqlite::params;

#[derive(Debug, Clone)]
pub struct Hit {
    pub slug: String,
    pub title: String,
    pub kind: String,
    pub body: String,
    pub score: f32,
    /// Set when the note was pulled in as a neighbour rather than matched
    /// directly, so the UI can show *why* it is in context.
    pub via: Option<String>,
}

fn cosine(a: &[f32], b: &[f32]) -> f32 {
    if a.len() != b.len() {
        return 0.0;
    }
    let (mut dot, mut na, mut nb) = (0.0, 0.0, 0.0);
    for (x, y) in a.iter().zip(b) {
        dot += x * y;
        na += x * x;
        nb += y * y;
    }
    if na == 0.0 || nb == 0.0 {
        return 0.0;
    }
    dot / (na.sqrt() * nb.sqrt())
}

impl Index {
    pub fn search(&self, provider: &Provider, model: &str, query: &str, k: usize) -> Result<Vec<Hit>> {
        let q = provider
            .embed(model, &[query.to_string()])?
            .into_iter()
            .next()
            .unwrap_or_default();

        let mut stmt = self.conn.prepare(
            "SELECT n.slug, n.title, n.kind, n.body, e.vec
             FROM embeddings e JOIN notes n ON n.slug = e.slug",
        )?;

        let mut hits: Vec<Hit> = stmt
            .query_map([], |r| {
                let vec: Vec<u8> = r.get(4)?;
                Ok(Hit {
                    slug: r.get(0)?,
                    title: r.get(1)?,
                    kind: r.get(2)?,
                    body: r.get(3)?,
                    score: cosine(&q, &blob_to_f32(&vec)),
                    via: None,
                })
            })?
            .collect::<Result<Vec<_>, _>>()?;

        hits.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
        hits.truncate(k);
        Ok(hits)
    }

    /// Pull in notes linked from the top hits. This is the payoff of keeping a
    /// graph at all: asking about a project surfaces the people on it even when
    /// their notes share no vocabulary with the question.
    pub fn expand(&self, hits: &[Hit], min_conf: f32, limit: usize) -> Result<Vec<Hit>> {
        let mut out = Vec::new();

        for hit in hits {
            let mut stmt = self.conn.prepare(
                "SELECT n.slug, n.title, n.kind, n.body, e.pred
                 FROM edges e JOIN notes n ON n.slug = e.obj OR n.slug LIKE '%/' || e.obj
                 WHERE e.src_slug = ?1 AND e.conf >= ?2",
            )?;

            let rows = stmt.query_map(params![hit.slug, min_conf], |r| {
                Ok(Hit {
                    slug: r.get(0)?,
                    title: r.get(1)?,
                    kind: r.get(2)?,
                    body: r.get(3)?,
                    score: hit.score * 0.5,
                    via: Some(format!("{} —{}→", hit.title, r.get::<_, String>(4)?)),
                })
            })?;

            for row in rows {
                let row = row?;
                let already = hits.iter().any(|h| h.slug == row.slug) || out.iter().any(|h: &Hit| h.slug == row.slug);
                if !already && out.len() < limit {
                    out.push(row);
                }
            }
        }
        Ok(out)
    }

    /// Retrieval + generation. Context is capped by character budget rather
    /// than token count so this stays honest on a 4k-context local model.
    pub fn ask(
        &self,
        provider: &Provider,
        embed_model: &str,
        chat_model: &str,
        question: &str,
        k: usize,
        budget: usize,
    ) -> Result<(String, Vec<Hit>)> {
        let mut hits = self.search(provider, embed_model, question, k)?;
        hits.extend(self.expand(&hits, 0.6, k / 2)?);

        let mut context = String::new();
        for h in &hits {
            let chunk = format!("## {} [{}]\n{}\n\n", h.title, h.slug, h.body.trim());
            if context.len() + chunk.len() > budget {
                break;
            }
            context.push_str(&chunk);
        }

        if context.is_empty() {
            return Ok(("Nothing in the vault touches that yet.".into(), hits));
        }

        let system = "You answer strictly from the provided vault notes. \
             Cite the note slug in square brackets after each claim. \
             If the notes do not contain the answer, say so plainly rather than guessing.";
        let user = format!("Notes:\n\n{context}\n---\n\nQuestion: {question}");

        let answer = provider.chat(chat_model, system, &user, None)?;
        Ok((answer, hits))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cosine_is_sane() {
        assert!((cosine(&[1.0, 0.0], &[1.0, 0.0]) - 1.0).abs() < 1e-6);
        assert!(cosine(&[1.0, 0.0], &[0.0, 1.0]).abs() < 1e-6);
        assert_eq!(cosine(&[1.0], &[1.0, 2.0]), 0.0, "mismatched dims must not panic");
        assert_eq!(cosine(&[0.0, 0.0], &[1.0, 1.0]), 0.0, "zero vector must not divide by zero");
    }
}
