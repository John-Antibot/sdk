// Command account_balance prints the balance of the account of your API key, in dollars.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/John-Avenue/john-antibot-sdk/account"
)

func main() {
	client, err := account.New(account.Config{APIKey: os.Getenv("JOHNANTIBOT_API_KEY"), Endpoint: os.Getenv("JOHNANTIBOT_API")})
	if err != nil {
		log.Fatalf("FAIL client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	balance, err := client.Balance(ctx)
	if err != nil {
		log.Fatalf("FAIL balance: %v", err)
	}
	if !balance.Metered {
		fmt.Println("PASS this key is not metered: it has no balance")
		return
	}
	fmt.Printf("PASS balance $%.2f (%d micros)\n", balance.Dollars(), balance.Micros)
}
