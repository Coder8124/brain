use anyhow::{Context, Result, bail};
use brain_core::{Index, Provider};
use std::path::PathBuf;

const EMBED_MODEL: &str = "nomic-embed-text";
const CHAT_MODEL: &str = "qwen3.6";

fn usage() -> ! {
    eprintln!(
        "brain — local-first second brain (step 1: index + retrieval)

USAGE
    brain doctor                 probe local model runtimes
    brain index [--watch]        sync vault into the cache and embed
    brain ask <question…>        retrieve and answer from the vault
    brain search <query…>        retrieve only, no generation

ENV
    BRAIN_VAULT     path to the vault (default ./vault)
    BRAIN_MODEL     chat model (default {CHAT_MODEL})
    BRAIN_EMBED     embed model (default {EMBED_MODEL})"
    );
    std::process::exit(2)
}

fn vault_path() -> PathBuf {
    std::env::var("BRAIN_VAULT").map(PathBuf::from).unwrap_or_else(|_| PathBuf::from("vault"))
}

fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

/// Pick the first running local runtime. Cloud BYOK slots in here later by
/// reading a configured base URL and key instead.
fn provider() -> Result<Provider> {
    let found = Provider::discover();
    let (p, models) = found
        .into_iter()
        .next()
        .context("no local model runtime found — start Ollama, LM Studio, Jan or Msty")?;
    eprintln!("· {} at {} ({} models)", p.name, p.base_url, models.len());
    Ok(p)
}

fn main() -> Result<()> {
    let args: Vec<String> = std::env::args().skip(1).collect();
    let Some(cmd) = args.first().map(String::as_str) else { usage() };
    let rest = args[1..].join(" ");

    match cmd {
        "doctor" => doctor(),
        "index" => index(args.iter().any(|a| a == "--watch")),
        "search" if !rest.is_empty() => search(&rest),
        "ask" if !rest.is_empty() => ask(&rest),
        _ => usage(),
    }
}

fn doctor() -> Result<()> {
    let found = Provider::discover();
    if found.is_empty() {
        bail!("no local runtime responding on any known port");
    }
    for (p, models) in found {
        println!("{} — {}", p.name, p.base_url);
        for m in models {
            println!("    {m}");
        }
    }
    Ok(())
}

fn open() -> Result<(Index, Provider)> {
    let vault = vault_path();
    if !vault.exists() {
        bail!("vault not found at {} — set BRAIN_VAULT", vault.display());
    }
    Ok((Index::open(&vault)?, provider()?))
}

fn index(watch: bool) -> Result<()> {
    let (mut idx, p) = open()?;
    let embed_model = env_or("BRAIN_EMBED", EMBED_MODEL);

    let run = |idx: &mut Index| -> Result<()> {
        let r = idx.sync()?;
        let embedded = idx.embed_pending(&p, &embed_model, 32)?;
        println!(
            "+{} ~{} -{} ={} · embedded {} · {} notes, {} edges",
            r.added,
            r.updated,
            r.removed,
            r.unchanged,
            embedded,
            idx.note_count()?,
            idx.edge_count()?
        );
        Ok(())
    };

    run(&mut idx)?;
    if !watch {
        return Ok(());
    }

    use notify::{EventKind, RecursiveMode, Watcher};
    let (tx, rx) = std::sync::mpsc::channel();
    let mut watcher = notify::recommended_watcher(tx)?;
    watcher.watch(&idx.vault, RecursiveMode::Recursive)?;
    println!("watching {} …", idx.vault.display());

    for event in &rx {
        let Ok(event) = event else { continue };
        // Ignore our own writes into .brain, or the watcher feeds itself.
        let touches_vault = event
            .paths
            .iter()
            .any(|p| p.extension().is_some_and(|e| e == "md") && !p.components().any(|c| c.as_os_str() == ".brain"));

        if touches_vault && matches!(event.kind, EventKind::Create(_) | EventKind::Modify(_) | EventKind::Remove(_)) {
            // Coalesce the burst an editor emits when saving a single file.
            std::thread::sleep(std::time::Duration::from_millis(300));
            while rx.try_recv().is_ok() {}
            run(&mut idx)?;
        }
    }
    Ok(())
}

fn search(query: &str) -> Result<()> {
    let (idx, p) = open()?;
    let hits = idx.search(&p, &env_or("BRAIN_EMBED", EMBED_MODEL), query, 8)?;
    for h in hits {
        println!("{:.3}  {:<28} {}", h.score, h.slug, h.title);
    }
    Ok(())
}

fn ask(question: &str) -> Result<()> {
    let (idx, p) = open()?;
    let (answer, hits) = idx.ask(
        &p,
        &env_or("BRAIN_EMBED", EMBED_MODEL),
        &env_or("BRAIN_MODEL", CHAT_MODEL),
        question,
        6,
        6000,
    )?;

    println!("\n{}\n", answer.trim());
    println!("─── context ───");
    for h in hits {
        match h.via {
            Some(via) => println!("  {:<28} via {via}", h.slug),
            None => println!("  {:<28} {:.3}", h.slug, h.score),
        }
    }
    Ok(())
}
