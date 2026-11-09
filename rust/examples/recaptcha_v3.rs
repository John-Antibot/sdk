//! Makes a reCAPTCHA v3 token through your proxy (`--enterprise` for an Enterprise page).
//!
//!     cargo run --example recaptcha_v3 -- https://www.example.com/login 6Lc... http://user:pass@host:port [action] [--enterprise]
use john_antibot::recaptcha;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let key = std::env::var("JOHNANTIBOT_API_KEY")?;
    let enterprise = std::env::args().any(|a| a == "--enterprise");
    let mut args = std::env::args().skip(1).filter(|a| a != "--enterprise");
    let usage = "usage: recaptcha_v3 <page URL> <site key> <proxy> [action] [--enterprise]";
    let input = recaptcha::Input {
        url: args.next().expect(usage),
        site_key: args.next().expect(usage),
        proxy: args.next().expect(usage),
        action: args.next(),
        ..Default::default()
    };

    let rc = recaptcha::Client::new(&key);
    let res = if enterprise {
        rc.solve_v3_enterprise(input).await?
    } else {
        rc.solve_v3(input).await?
    };
    println!(
        "token: {}\nexpires: {}",
        res.token,
        res.expires_at.as_deref().unwrap_or("-")
    );
    for (name, value) in &res.headers() {
        println!("{name}: {}", value.to_str().unwrap_or_default());
    }
    Ok(())
}
