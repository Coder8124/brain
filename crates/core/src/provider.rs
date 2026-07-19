//! One client for every runtime.
//!
//! Ollama, LM Studio, Jan and Msty all speak OpenAI-compatible `/v1`, so the
//! only thing that differs is the port and which models happen to be loaded.
//! Cloud BYOK is the same surface with a base URL and a key.

use anyhow::{Context, Result, bail};
use serde::Deserialize;
use serde_json::json;
use std::time::Duration;

/// Known local runtimes, in the order we probe them.
pub const LOCAL_ENDPOINTS: &[(&str, &str)] = &[
    ("Ollama", "http://localhost:11434/v1"),
    ("LM Studio", "http://localhost:1234/v1"),
    ("Jan", "http://localhost:1337/v1"),
    ("Msty", "http://localhost:10000/v1"),
];

#[derive(Debug, Clone)]
pub struct Provider {
    pub name: String,
    pub base_url: String,
    pub api_key: Option<String>,
    http: reqwest::blocking::Client,
}

#[derive(Deserialize)]
struct ModelList {
    data: Vec<ModelEntry>,
}

#[derive(Deserialize)]
struct ModelEntry {
    id: String,
}

#[derive(Deserialize)]
struct EmbeddingResponse {
    data: Vec<EmbeddingEntry>,
}

#[derive(Deserialize)]
struct EmbeddingEntry {
    embedding: Vec<f32>,
}

#[derive(Deserialize)]
struct ChatResponse {
    choices: Vec<Choice>,
}

#[derive(Deserialize)]
struct Choice {
    message: ChatMessage,
}

#[derive(Deserialize)]
struct ChatMessage {
    content: String,
}

impl Provider {
    pub fn new(name: impl Into<String>, base_url: impl Into<String>, api_key: Option<String>) -> Self {
        Provider {
            name: name.into(),
            base_url: base_url.into().trim_end_matches('/').to_string(),
            api_key,
            http: reqwest::blocking::Client::builder()
                // Local generation on a 24B model can genuinely take a while.
                .timeout(Duration::from_secs(300))
                .build()
                .expect("http client"),
        }
    }

    /// Probe the well-known ports and return whatever is actually running.
    /// This is what lets the app say "found LM Studio running Qwen3" on first
    /// launch instead of asking the user to configure an endpoint.
    pub fn discover() -> Vec<(Provider, Vec<String>)> {
        let probe = reqwest::blocking::Client::builder()
            .timeout(Duration::from_millis(400))
            .build()
            .expect("http client");

        LOCAL_ENDPOINTS
            .iter()
            .filter_map(|(name, url)| {
                let list: ModelList = probe.get(format!("{url}/models")).send().ok()?.json().ok()?;
                let models = list.data.into_iter().map(|m| m.id).collect();
                Some((Provider::new(*name, *url, None), models))
            })
            .collect()
    }

    fn post(&self, path: &str, body: serde_json::Value) -> Result<reqwest::blocking::Response> {
        let mut req = self.http.post(format!("{}{path}", self.base_url)).json(&body);
        if let Some(key) = &self.api_key {
            req = req.bearer_auth(key);
        }
        let res = req
            .send()
            .with_context(|| format!("{} unreachable at {}", self.name, self.base_url))?;

        if !res.status().is_success() {
            let status = res.status();
            let detail = res.text().unwrap_or_default();
            bail!("{} returned {status}: {}", self.name, detail.trim());
        }
        Ok(res)
    }

    pub fn embed(&self, model: &str, inputs: &[String]) -> Result<Vec<Vec<f32>>> {
        if inputs.is_empty() {
            return Ok(vec![]);
        }
        let res: EmbeddingResponse = self
            .post("/embeddings", json!({ "model": model, "input": inputs }))?
            .json()
            .context("malformed embedding response")?;

        if res.data.len() != inputs.len() {
            bail!("expected {} embeddings, got {}", inputs.len(), res.data.len());
        }
        Ok(res.data.into_iter().map(|e| e.embedding).collect())
    }

    /// `schema` enables constrained decoding. Small local models are unreliable
    /// at tool-calling but near-perfect when JSON is enforced at the sampler,
    /// which is why every extraction path in this project goes through here.
    pub fn chat(
        &self,
        model: &str,
        system: &str,
        user: &str,
        schema: Option<serde_json::Value>,
    ) -> Result<String> {
        let mut body = json!({
            "model": model,
            "messages": [
                { "role": "system", "content": system },
                { "role": "user", "content": user },
            ],
            "temperature": 0.2,
            "stream": false,
        });

        if let Some(schema) = schema {
            body["response_format"] = json!({
                "type": "json_schema",
                "json_schema": { "name": "out", "strict": true, "schema": schema },
            });
        }

        let res: ChatResponse = self.post("/chat/completions", body)?.json().context("malformed chat response")?;
        res.choices
            .into_iter()
            .next()
            .map(|c| c.message.content)
            .context("no choices returned")
    }
}
