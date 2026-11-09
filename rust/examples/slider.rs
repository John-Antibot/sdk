//! Makes the slider check URL for a captcha page you loaded.
//!
//!     cargo run --example slider -- page.html 'https://geo.captcha-delivery.com/captcha/?...' [script.js]
use john_antibot::datadome;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let key = std::env::var("JOHNANTIBOT_API_KEY")?;
    let mut args = std::env::args().skip(1);
    let usage = "usage: slider <page.html> <captcha URL> [script.js]";
    let html = std::fs::read_to_string(args.next().expect(usage))?;
    let captcha_url = args.next().expect(usage);
    let script = match (datadome::challenge_script_url(&html), args.next()) {
        (Some(url), None) => {
            return Err(format!("load {url} right after the page and pass the file").into());
        }
        (Some(_), Some(path)) => Some(std::fs::read_to_string(path)?),
        (None, _) => None,
    };

    let mut session = datadome::Client::new(&key).session(datadome::SessionOptions::default());
    match session
        .generate_slider(datadome::SliderInput {
            html,
            captcha_url,
            script,
            ..Default::default()
        })
        .await
    {
        Ok(slider) => println!(
            "GET {}\nuser-agent: {}",
            slider.check_url,
            session.user_agent()
        ),
        Err(e) if e.is_hardblock() => println!("hardblock: change the proxy and the session"),
        Err(e) if e.is_puzzle_unsupported() => println!("a puzzle captcha, not a slider"),
        Err(e) => return Err(e.into()),
    }
    Ok(())
}
