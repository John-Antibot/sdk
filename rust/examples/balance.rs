//! Prints the balance of your key's account.
//!
//!     cargo run --example balance
use john_antibot::account;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let key = std::env::var("JOHNANTIBOT_API_KEY")?;
    let balance = account::Client::new(&key).balance().await?;
    if balance.metered {
        println!("${:.4}", balance.balance_usd());
    } else {
        println!("not metered");
    }
    Ok(())
}
