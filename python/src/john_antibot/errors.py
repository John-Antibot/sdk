"""The errors the clients raise, and the predicates that tell them apart."""

from __future__ import annotations


class JohnAntibotError(Exception):
    """Base of ApiError and JobError; raised as is when the API answers 2xx with a reply the client cannot read."""


class ApiError(JohnAntibotError):
    """A non-2xx reply. message is the API's error text (the JSON ``error``, else the body, else the status text)."""

    def __init__(self, status: int, message: str, *, hardblock: bool = False, service: str = "") -> None:
        self.status = status
        self.message = message
        # The site blocked the session outright (DataDome's hardblock page, no slider to solve): 422, nothing charged.
        self.hardblock = hardblock
        self.service = service
        super().__init__(f"{service}: HTTP {status}: {message}")


class JobError(JohnAntibotError):
    """A job that ended without a result: state is ``failed`` (the price is returned) or ``declined`` (not charged: send another)."""

    def __init__(self, id: str, state: str, message: str, *, retry: bool = False, service: str = "") -> None:
        self.id = id
        self.state = state
        self.message = message
        self.retry = retry
        self.service = service
        super().__init__(f"{service}: job {id} {state}: {message}")


def _status(err: BaseException, status: int) -> bool:
    return isinstance(err, ApiError) and err.status == status


def is_hardblock(err: BaseException) -> bool:
    """DataDome served its hardblock page (422, ``"hardblock": true``): change the proxy and the session. Free."""
    return isinstance(err, ApiError) and (err.hardblock or "hardblock" in err.message)


def is_puzzle_unsupported(err: BaseException) -> bool:
    """DataDome served a puzzle captcha, not a slider. Free."""
    return isinstance(err, ApiError) and "puzzle slider unsupported" in err.message


def is_rate_limited(err: BaseException) -> bool:
    """HTTP 429: wait, then try again."""
    return _status(err, 429)


def is_insufficient_balance(err: BaseException) -> bool:
    """HTTP 402: the balance is below the price."""
    return _status(err, 402)


def is_site_not_approved(err: BaseException) -> bool:
    """HTTP 403: the site is not approved for the key."""
    return _status(err, 403)


def is_bad_request(err: BaseException) -> bool:
    """HTTP 400: the input was refused."""
    return _status(err, 400)


def is_bad_key(err: BaseException) -> bool:
    """HTTP 401: the key is missing, unknown or revoked, or its account is disabled."""
    return _status(err, 401)


def is_declined(err: BaseException) -> bool:
    """The job was declined: the attempt was turned down and not charged. Send another."""
    return isinstance(err, JobError) and err.state == "declined"


def is_no_challenge(err: BaseException) -> bool:
    """The page serves no Cloudflare challenge: there is nothing to clear."""
    return isinstance(err, JobError) and "serves no challenge" in err.message


def is_rotating_proxy(err: BaseException) -> bool:
    """The proxy exits from a different address on each connection. Use a sticky session or a static proxy."""
    return isinstance(err, JobError) and "different address on each connection" in err.message
