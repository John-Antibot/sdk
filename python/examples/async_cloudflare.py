"""Read the balance, then clear several pages at once with asyncio.

    PROXY=http://user:pass@host:port python examples/async_cloudflare.py https://www.example.com/ https://www.example.org/
"""

import asyncio
import os
import sys

from john_antibot import AsyncAccount, AsyncCloudflare


async def main() -> None:
    key, proxy, urls = os.environ["JOHNANTIBOT_API_KEY"], os.environ["PROXY"], sys.argv[1:]
    async with AsyncAccount(api_key=key) as account, AsyncCloudflare(api_key=key) as cf:
        print(f"balance ${(await account.balance()).balance_usd:.2f}")
        solves = [cf.solve_challenge(url=url, proxy=proxy, max_wait=120) for url in urls]
        for url, res in zip(urls, await asyncio.gather(*solves, return_exceptions=True)):
            print(url, res if isinstance(res, BaseException) else res.clearance)


asyncio.run(main())
