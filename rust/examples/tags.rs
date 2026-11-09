//! Makes the `ch` tags payload for a page.
//!
//!     cargo run --example tags -- https://www.example.com/ <datadome cookie> <site key> [font family]...
use john_antibot::datadome;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let key = std::env::var("JOHNANTIBOT_API_KEY")?;
    let mut args = std::env::args().skip(1);
    let usage = "usage: tags <page URL> <datadome cookie> <site key> [font family]...";
    let (page, cookie, site_key) = (
        args.next().expect(usage),
        args.next().expect(usage),
        args.next().expect(usage),
    );

    let client = datadome::Client::new(&key);
    let mut session = client.session(datadome::SessionOptions {
        referer: Some(page),
        ..Default::default()
    });
    let tags = session
        .generate_tags(datadome::TagsInput {
            event: datadome::Event::Ch,
            cookie,
            site_key,
            // the @font-face families the page loaded (document.fonts)
            loaded_font_families: args.collect(),
            ..Default::default()
        })
        .await?;
    // POST it to the site's /js/ with this user agent, then make `le` with the cookie the reply sets.
    println!(
        "payload: {}\nuser-agent: {}",
        tags.payload,
        session.user_agent()
    );
    Ok(())
}
