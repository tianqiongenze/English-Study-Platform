"""ASGI entrypoint for Granian (ADR-09). Usage: granian app.asgi:app --interface asgi."""
from app.main import app

__all__ = ["app"]
