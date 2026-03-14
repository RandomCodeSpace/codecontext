from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Protocol

import httpx


ProviderType = str
PROVIDER_OLLAMA: ProviderType = "ollama"
PROVIDER_AZURE: ProviderType = "azure"
PROVIDER_OPENAI: ProviderType = "openai"
PROVIDER_MOCK: ProviderType = "mock"
@dataclass
class Config:
    provider: ProviderType
    model: str
    temperature: float
    max_tokens: int
    timeout_seconds: int
    ollama_url: str
    azure_endpoint: str
    azure_key: str
    azure_api_version: str
    openai_api_key: str

    def validate(self) -> None:
        if self.provider == PROVIDER_OLLAMA and not self.ollama_url:
            raise ValueError("OLLAMA_BASE_URL not set")
        if self.provider == PROVIDER_AZURE:
            if not self.azure_endpoint:
                raise ValueError("AZURE_OPENAI_ENDPOINT not set")
            if not self.azure_key:
                raise ValueError("AZURE_OPENAI_KEY not set")
        if self.provider == PROVIDER_OPENAI and not self.openai_api_key:
            raise ValueError("OPENAI_API_KEY not set")
        if self.provider not in {PROVIDER_OLLAMA, PROVIDER_AZURE, PROVIDER_OPENAI, PROVIDER_MOCK}:
            raise ValueError(f"unknown provider: {self.provider}")
        if not self.model:
            raise ValueError("model not set")
        if self.temperature < 0 or self.temperature > 2:
            raise ValueError("temperature must be between 0 and 2")
        if self.max_tokens < 1:
            raise ValueError("max_tokens must be at least 1")


@dataclass
class Message:
    role: str
    content: str


class Provider(Protocol):
    def complete(self, prompt: str) -> str: ...
    def chat(self, messages: list[Message]) -> str: ...
    def is_healthy(self) -> tuple[bool, str]: ...
    def get_provider(self) -> ProviderType: ...


def load_config() -> Config:
    _load_env_file(".env.local")
    _load_env_file(".env")

    provider = os.getenv("LLM_PROVIDER", PROVIDER_OLLAMA)
    model = os.getenv("LLM_MODEL", "llama2")

    openai_model = os.getenv("OPENAI_MODEL")
    azure_deployment = os.getenv("AZURE_OPENAI_DEPLOYMENT")
    if provider == PROVIDER_OPENAI and openai_model:
        model = openai_model
    if provider == PROVIDER_AZURE and azure_deployment:
        model = azure_deployment

    return Config(
        provider=provider,
        model=model,
        temperature=float(os.getenv("LLM_TEMPERATURE", "0.7")),
        max_tokens=int(os.getenv("LLM_MAX_TOKENS", "2000")),
        timeout_seconds=int(os.getenv("LLM_TIMEOUT_SECONDS", "30")),
        ollama_url=os.getenv("OLLAMA_BASE_URL", "http://localhost:11434"),
        azure_endpoint=os.getenv("AZURE_OPENAI_ENDPOINT", ""),
        azure_key=os.getenv("AZURE_OPENAI_KEY", ""),
        azure_api_version=os.getenv("AZURE_OPENAI_API_VERSION", "2024-02-15-preview"),
        openai_api_key=os.getenv("OPENAI_API_KEY", ""),
    )


def new_provider(cfg: Config) -> Provider:
    cfg.validate()
    if cfg.provider == PROVIDER_OLLAMA:
        return OllamaProvider(cfg)
    if cfg.provider == PROVIDER_OPENAI:
        return OpenAIProvider(cfg)
    if cfg.provider == PROVIDER_AZURE:
        return AzureProvider(cfg)
    return MockProvider(cfg)


class MockProvider:
    def __init__(self, cfg: Config):
        self.cfg = cfg

    def complete(self, prompt: str) -> str:
        return f"[mock:{self.cfg.model}] {prompt[:200]}"

    def chat(self, messages: list[Message]) -> str:
        user = next((m.content for m in reversed(messages) if m.role == "user"), "")
        return f"[mock:{self.cfg.model}] {user[:400]}"

    def is_healthy(self) -> tuple[bool, str]:
        return (True, "ok")

    def get_provider(self) -> ProviderType:
        return PROVIDER_MOCK


class OllamaProvider:
    def __init__(self, cfg: Config):
        self.cfg = cfg
        self.client = httpx.Client(timeout=cfg.timeout_seconds)

    def complete(self, prompt: str) -> str:
        payload = {
            "model": self.cfg.model,
            "prompt": prompt,
            "stream": False,
            "temperature": self.cfg.temperature,
            "num_predict": self.cfg.max_tokens,
        }
        r = self.client.post(f"{self.cfg.ollama_url}/api/generate", json=payload)
        r.raise_for_status()
        data = r.json()
        return str(data.get("response", ""))

    def chat(self, messages: list[Message]) -> str:
        payload = {
            "model": self.cfg.model,
            "messages": [{"role": m.role, "content": m.content} for m in messages],
            "stream": False,
            "temperature": self.cfg.temperature,
            "num_predict": self.cfg.max_tokens,
        }
        r = self.client.post(f"{self.cfg.ollama_url}/api/chat", json=payload)
        r.raise_for_status()
        data = r.json()
        msg = data.get("message", {})
        return str(msg.get("content", ""))

    def is_healthy(self) -> tuple[bool, str]:
        try:
            r = self.client.get(f"{self.cfg.ollama_url}/api/tags")
            if r.status_code == 200:
                return (True, "ok")
            return (False, f"status {r.status_code}")
        except Exception as err:  # noqa: BLE001
            return (False, str(err))

    def get_provider(self) -> ProviderType:
        return PROVIDER_OLLAMA


class OpenAIProvider:
    def __init__(self, cfg: Config):
        self.cfg = cfg

    def complete(self, prompt: str) -> str:
        from openai import OpenAI

        client = OpenAI(api_key=self.cfg.openai_api_key, timeout=self.cfg.timeout_seconds)
        resp = client.chat.completions.create(
            model=self.cfg.model,
            messages=[{"role": "user", "content": prompt}],
            temperature=self.cfg.temperature,
            max_tokens=self.cfg.max_tokens,
        )
        return resp.choices[0].message.content or ""

    def chat(self, messages: list[Message]) -> str:
        from openai import OpenAI

        client = OpenAI(api_key=self.cfg.openai_api_key, timeout=self.cfg.timeout_seconds)
        resp = client.chat.completions.create(
            model=self.cfg.model,
            messages=[{"role": m.role, "content": m.content} for m in messages],
            temperature=self.cfg.temperature,
            max_tokens=self.cfg.max_tokens,
        )
        return resp.choices[0].message.content or ""

    def is_healthy(self) -> tuple[bool, str]:
        try:
            _ = self.complete("ping")
            return (True, "ok")
        except Exception as err:  # noqa: BLE001
            return (False, str(err))

    def get_provider(self) -> ProviderType:
        return PROVIDER_OPENAI


class AzureProvider:
    def __init__(self, cfg: Config):
        self.cfg = cfg

    def complete(self, prompt: str) -> str:
        return self.chat([Message(role="user", content=prompt)])

    def chat(self, messages: list[Message]) -> str:
        from openai import AzureOpenAI

        client = AzureOpenAI(
            api_key=self.cfg.azure_key,
            azure_endpoint=self.cfg.azure_endpoint,
            api_version=self.cfg.azure_api_version,
            timeout=self.cfg.timeout_seconds,
        )
        resp = client.chat.completions.create(
            model=self.cfg.model,
            messages=[{"role": m.role, "content": m.content} for m in messages],
            temperature=self.cfg.temperature,
            max_tokens=self.cfg.max_tokens,
        )
        return resp.choices[0].message.content or ""

    def is_healthy(self) -> tuple[bool, str]:
        try:
            _ = self.complete("ping")
            return (True, "ok")
        except Exception as err:  # noqa: BLE001
            return (False, str(err))

    def get_provider(self) -> ProviderType:
        return PROVIDER_AZURE


def _load_env_file(path: str) -> None:
    p = Path(path)
    if not p.exists():
        return
    for raw in p.read_text(encoding="utf-8", errors="replace").splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        os.environ.setdefault(key, value)
