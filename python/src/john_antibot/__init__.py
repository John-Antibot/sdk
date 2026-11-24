"""Python client for the John Antibot DataDome, Cloudflare, reCAPTCHA and Kasada APIs and your account's balance."""

from ._http import DEFAULT_ENDPOINT
from .account import Account, AsyncAccount
from .cloudflare import AsyncCloudflare, Cloudflare
from .datadome import AsyncDataDome, DataDome
from .recaptcha import AsyncRecaptcha, Recaptcha
from .kasada import AsyncKasada, Kasada
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

__version__ = "0.9.1"

__all__ = [
    "DEFAULT_ENDPOINT",
    "Account",
    "ApiError",
    "AsyncAccount",
    "AsyncCloudflare",
    "AsyncDataDome",
    "AsyncRecaptcha",
    "AsyncKasada",
    "Cloudflare",
    "DataDome",
    "JobError",
    "JohnAntibotError",
    "Recaptcha",
    "Kasada",
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
