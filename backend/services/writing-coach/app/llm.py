"""Writing Coach: LLM provider for improvement plan generation."""
from __future__ import annotations
import os


def new_provider():
    """Return a callable(prompt, max_tokens) -> str. Mock if no key."""
    key = os.getenv("ANTHROPIC_API_KEY", "")
    mode = os.getenv("LLM_MODE", "mock")
    if mode == "real" and key:
        try:
            import anthropic  # type: ignore
            client = anthropic.Anthropic(api_key=key)

            def _real(prompt: str, max_tokens: int = 1024) -> str:
                resp = client.messages.create(
                    model="claude-3-5-sonnet-20241022",
                    max_tokens=max_tokens,
                    messages=[{"role": "user", "content": prompt}],
                )
                return resp.content[0].text  # type: ignore
            return _real
        except Exception:
            pass

    def _mock(prompt: str, max_tokens: int = 1024) -> str:
        return (
            "【写作能力提升方案(Mock)】\n"
            "阶段1(1-2周):夯实语法——每日 1 篇改错练习,聚焦主谓一致/时态。\n"
            "阶段2(3-4周):段落连贯——学习 topic sentence + 过渡词,每两天写 1 段。\n"
            "阶段3(5-6周):词汇升级——替换 20 个高频词为学术词;每周完成 1 篇限时作文。\n"
            "阶段4(7-8周):全真模拟——按考试时间完成整套写作,结合批改迭代。\n"
        )
    return _mock
