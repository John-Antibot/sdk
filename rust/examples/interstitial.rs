//! Makes the interstitial form for a page you loaded.
//!
//!     cargo run --example interstitial -- page.html 'https://geo.captcha-delivery.com/interstitial/?...' [script.js]
use john_antibot::datadome;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let key = std::env::var("JOHNANTIBOT_API_KEY")?;
    let mut args = std::env::args().skip(1);
    let usage = "usage: interstitial <page.html> <interstitial URL> [script.js]";
    let html = std::fs::read_to_string(args.next().expect(usage))?;
    let device_link = args.next().expect(usage);
    // The script the page loads (<script src>), loaded right after the page; none when it is inline.
    let script = match (datadome::challenge_script_url(&html), args.next()) {
        (Some(url), None) => {
            return Err(format!("load {url} right after the page and pass the file").into());
        }
        (Some(_), Some(path)) => Some(std::fs::read_to_string(path)?),
        (None, _) => None,
    };

    let mut session = datadome::Client::new(&key).session(datadome::SessionOptions::default());
    let inter = session
        .generate_interstitial(datadome::InterstitialInput {
            html,
            device_link,
            script,
            ..Default::default()
        })
        .await?;
    println!(
        "POST {}/interstitial/\n{}\nuser-agent: {}",
        datadome::GEO_ORIGIN,
        inter.form_body,
        session.user_agent()
    );
    Ok(())
}
