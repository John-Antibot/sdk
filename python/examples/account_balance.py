"""Print the balance of your key's account.

    python examples/account_balance.py
"""

import os

from john_antibot import Account

with Account(api_key=os.environ["JOHNANTIBOT_API_KEY"]) as account:
    balance = account.balance()

print(f"${balance.balance_usd:.2f} ({balance.balance_micros} micros)" if balance.metered else "this key is not metered: it has no balance")
