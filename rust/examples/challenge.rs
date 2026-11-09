//! Clears a Cloudflare challenge page through your proxy.
//!
//!     cargo run --example challenge -- https://www.example.com/ http://user:pass@host:port [America/New_York]
use john_antibot::cloudflare;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let key = std::env::var("JOHNANTIBOT_API_KEY")?;
    let mut args = std::env::args().skip(1);
    let usage = "usage: challenge <page URL> <proxy> [timezone]";
    let (url, proxy, timezone) = (
        args.next().expect(usage),
        args.next().expect(usage),
        args.next(),
    );

    let cf = cloudflare::Client::new(&key);
    let res = cf
        .solve_challenge(cloudflare::ChallengeInput {
            url,
            proxy,
            timezone,
            ..Default::default()
        })
        .await?;
    println!("cf_clearance: {}", res.clearance().unwrap_or_default());
    println!("user-agent: {}\nexit IP: {}", res.user_agent, res.exit_ip);
    for c in &res.cookies {
        println!("cookie: {}={}", c.name, c.value);
    }
    Ok(())
}
