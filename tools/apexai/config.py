"""Settings from the environment, with an optional .env file for local development."""

import os
from dataclasses import dataclass
from pathlib import Path

DEFAULT_DB = "postgres://apex:apex@localhost:55432/apex?sslmode=disable"
DEFAULT_MODEL = "openai/gpt-oss-120b"


def load_dotenv(start: Path | None = None) -> None:
    """Read KEY=VALUE lines from the nearest .env upwards. Real environment variables win."""
    here = (start or Path.cwd()).resolve()
    for folder in (here, *here.parents):
        path = folder / ".env"
        if path.is_file():
            for line in path.read_text(encoding="utf-8").splitlines():
                line = line.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                key, value = line.split("=", 1)
                os.environ.setdefault(key.strip(), value.strip())
            return


@dataclass(frozen=True)
class Settings:
    db_url: str
    groq_api_key: str
    model: str

    @classmethod
    def from_env(cls) -> "Settings":
        load_dotenv()
        return cls(
            db_url=os.environ.get("APEX_DB_URL", DEFAULT_DB),
            groq_api_key=os.environ.get("GROQ_API_KEY", ""),
            model=os.environ.get("APEX_AI_MODEL", DEFAULT_MODEL),
        )
