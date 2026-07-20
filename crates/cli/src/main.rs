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
    brain capture [--daemon]     pull episodic events (--daemon also samples focus)
    brain timeline [--verbose]   today's activity
    brain prune [days]           drop raw events past the retention window

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
        "capture" => capture(args.iter().any(|a| a == "--daemon")),
        "timeline" => timeline(args.iter().any(|a| a == "--verbose")),
        "prune" => prune(args.get(1).and_then(|d| d.parse().ok()).unwrap_or(90)),
        _ => usage(),
    }
}

/// Repos to mine for commits. Defaults to the vault's parent so a single-repo
/// setup works with no configuration.
fn watched_repos() -> Vec<PathBuf> {
    std::env::var("BRAIN_REPOS")
        .map(|v| v.split(':').map(PathBuf::from).collect())
        .unwrap_or_else(|_| std::env::current_dir().into_iter().collect())
}

fn scratch_dir(vault: &std::path::Path) -> PathBuf {
    vault.join(".brain/scratch")
}

fn open_events() -> Result<(PathBuf, rusqlite::Connection)> {
    let vault = vault_path();
    let idx = Index::open(&vault)?;
    brain_capture::store::init(&idx.conn)?;
    Ok((vault, idx.conn))
}

fn capture(daemon: bool) -> Result<()> {
    let (vault, mut conn) = open_events()?;
    let policy = brain_capture::Policy::default();
    let repos = watched_repos();

    let n = brain_capture::poll_once(&mut conn, &scratch_dir(&vault), &repos, &policy)?;
    println!("+{n} events · {} total", brain_capture::store::count(&conn)?);

    if !daemon {
        return Ok(());
    }

    use brain_capture::sources::frontmost::{Frontmost, Granularity, POLL_INTERVAL};
    let front = Frontmost::probe();
    match front.granularity {
        Granularity::AppAndTitle => println!("· focus sampling: app + window title"),
        Granularity::AppOnly => println!(
            "· focus sampling: app name only — grant Accessibility to System Events for window titles"
        ),
    }
    println!("· recording. ^C to stop.");

    // Sessions are only written when they end, so a crash loses at most the
    // one in flight. That is the right trade against writing every 5s sample.
    let mut coalescer = brain_capture::Coalescer::new(60);
    let mut since_pull = 0u64;

    loop {
        if let Ok(sample) = front.sample() {
            if !policy.should_drop(&sample) {
                if let Some(done) = coalescer.push(sample) {
                    brain_capture::store::insert(&conn, &done)?;
                }
            }
        }

        std::thread::sleep(POLL_INTERVAL);
        since_pull += POLL_INTERVAL.as_secs();

        // Browser and git are pull sources; polling them every 5s would be
        // wasteful, so they run on their own slower cadence.
        if since_pull >= 300 {
            since_pull = 0;
            let n = brain_capture::poll_once(&mut conn, &scratch_dir(&vault), &repos, &policy)?;
            if n > 0 {
                println!("+{n} events");
            }
        }
    }
}

/// Local midnight, derived the same way the timeline renderer derives its offset.
fn today_bounds() -> (i64, i64) {
    let now = brain_capture::event::now();
    let offset = brain_capture::timeline::local_offset();
    let start = (now + offset).div_euclid(86_400) * 86_400 - offset;
    (start, start + 86_400)
}

fn timeline(verbose: bool) -> Result<()> {
    let (_, conn) = open_events()?;
    let (from, to) = today_bounds();
    let events = brain_capture::store::range(&conn, from, to)?;

    print!("{}", brain_capture::timeline::render(&events, verbose));

    let totals = brain_capture::timeline::by_app(&events);
    if !totals.is_empty() {
        println!("\n─── time by app ───");
        for (app, secs) in totals.iter().take(10) {
            println!("  {:<20} {}", app, brain_capture::timeline::dur(*secs));
        }
    }
    Ok(())
}

fn prune(days: i64) -> Result<()> {
    let (_, conn) = open_events()?;
    let cutoff = brain_capture::event::now() - days * 86_400;
    let n = brain_capture::store::prune(&conn, cutoff)?;
    println!("dropped {n} events older than {days}d · {} remain", brain_capture::store::count(&conn)?);
    Ok(())
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
