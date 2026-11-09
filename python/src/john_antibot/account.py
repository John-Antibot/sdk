"""Account: the balance your key is charged against."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Dict, Optional

import httpx

from ._http import DEFAULT_ENDPOINT, Call, Op, _Async, _Sync

__all__ = ["Account", "AsyncAccount", "Balance"]


@dataclass
class Balance:
    """What the account of a key has to spend."""

    metered: bool  # False: a key that is not metered has no account, and balance_micros is 0
    balance_micros: int  # millionths of a dollar, the unit of a price in the catalog (2000 is $0.002)

    @property
    def balance_usd(self) -> float:
        """The balance in dollars, for display."""
        return self.balance_micros / 1e6


def _balance() -> Op[Balance]:
    data: Dict[str, Any] = yield Call("GET", "/v1/balance")
    micros = data.get("balanceMicros")
    return Balance(metered=data.get("metered") is True, balance_micros=micros if isinstance(micros, int) and not isinstance(micros, bool) else 0)


class Account(_Sync):
    """Account client."""

    _service = "account"
    _default_timeout = 20.0

    def __init__(
        self, api_key: str, *, endpoint: str = DEFAULT_ENDPOINT, timeout: Optional[float] = None, http_client: Optional[httpx.Client] = None
    ) -> None:
        super().__init__(api_key, endpoint=endpoint, timeout=timeout, http_client=http_client)

    def balance(self) -> Balance:
        """The balance of the account of the key. Free."""
        return self._run(_balance())


class AsyncAccount(_Async):
    """Account client for asyncio."""

    _service = "account"
    _default_timeout = 20.0

    def __init__(
        self, api_key: str, *, endpoint: str = DEFAULT_ENDPOINT, timeout: Optional[float] = None, http_client: Optional[httpx.AsyncClient] = None
    ) -> None:
        super().__init__(api_key, endpoint=endpoint, timeout=timeout, http_client=http_client)

    async def balance(self) -> Balance:
        return await self._run(_balance())
