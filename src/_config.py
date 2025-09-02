from os import getenv
from typing import Optional

from dotenv import load_dotenv

load_dotenv()


def get_env_int(name: str, default: Optional[int] = None) -> Optional[int]:
    value = getenv(name)
    try:
        return int(value)
    except (TypeError, ValueError):
        print(f"Invalid value for {name}: {value}")
        return default

API_ID: Optional[int] = get_env_int("API_ID")
API_HASH: Optional[str] = getenv("API_HASH")
TOKEN: Optional[str] = getenv("TOKEN")
