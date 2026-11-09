//! Earns the JSD clearance of a page through your proxy.
//!
//!     cargo run --example jsd -- https://www.example.com/ http://user:pass@host:port
use john_antibot::cloudflare;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let key = std::env::var("JOHNANTIBOT_API_KEY")?;
    let mut args = std::env::args().skip(1);
    let usage = "usage: jsd <page URL> <proxy>";
    let (url, proxy) = (args.next().expect(usage), args.next().expect(usage));

    let cf = cloudflare::Client::new(&key);
    let res = cf
        .solve_jsd(cloudflare::JsdInput {
            url,
            proxy,
            ..Default::default()
        })
        .await?;
    println!(
        "cf_clearance: {}\nuser-agent: {}\nexit IP: {}",
        res.clearance().unwrap_or_default(),
        res.user_agent,
        res.exit_ip
    );
    Ok(())
}
