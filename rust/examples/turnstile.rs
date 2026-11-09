//! Solves a Turnstile widget through your proxy.
//!
//!     cargo run --example turnstile -- https://www.example.com/login 0x4AAAAAAA... http://user:pass@host:port
use john_antibot::cloudflare;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let key = std::env::var("JOHNANTIBOT_API_KEY")?;
    let mut args = std::env::args().skip(1);
    let usage = "usage: turnstile <page URL> <site key> <proxy>";
    let (page_url, site_key, proxy) = (
        args.next().expect(usage),
        args.next().expect(usage),
        args.next().expect(usage),
    );

    let cf = cloudflare::Client::new(&key);
    let res = cf
        .solve_turnstile(cloudflare::TurnstileInput {
            page_url,
            site_key,
            proxy,
            ..Default::default()
        })
        .await?;
    println!(
        "cf-turnstile-response: {}\nuser-agent: {}",
        res.token(),
        res.user_agent
    );
    Ok(())
}
