"""Python client for the John Antibot DataDome, Cloudflare and reCAPTCHA APIs and your account's balance."""

from ._http import DEFAULT_ENDPOINT
from .account import Account, AsyncAccount
from .cloudflare import AsyncCloudflare, Cloudflare
from .datadome import AsyncDataDome, DataDome
from .recaptcha import AsyncRecaptcha, Recaptcha
from .errors import (
    ApiError,
    JobError,
    JohnAntibotError,
    is_bad_key,
    is_bad_request,
    is_declined,
    is_hardblock,
    is_insufficient_balance,
    is_no_challenge,
    is_puzzle_unsupported,
    is_rate_limited,
    is_rotating_proxy,
    is_site_not_approved,
)

__version__ = "0.8.1"

__all__ = [
    "DEFAULT_ENDPOINT",
    "Account",
    "ApiError",
    "AsyncAccount",
    "AsyncCloudflare",
    "AsyncDataDome",
    "AsyncRecaptcha",
    "Cloudflare",
    "DataDome",
    "JobError",
    "JohnAntibotError",
    "Recaptcha",
    "is_bad_key",
    "is_bad_request",
    "is_declined",
    "is_hardblock",
    "is_insufficient_balance",
    "is_no_challenge",
    "is_puzzle_unsupported",
    "is_rate_limited",
    "is_rotating_proxy",
    "is_site_not_approved",
]
