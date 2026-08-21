# llm-cache-embedding 路由测试(SCS/CLC 铁律:验证 /embed 路由 + model 优先级 + 维度断言)
#
# 设计:不加载真实模型(重,会压 VM,见 memory esp-llm-cache-embedding-vm-snowball)。
# 用 monkeypatch 替换 _load_model 为返回固定 dim/encode 的 stub,
# 只测路由逻辑(_route_model / model 优先级 / 维度返回 / 错误处理)。
# 真实模型推理标 @pytest.mark.slow,默认跳过。
import os
import sys
import pytest
from fastapi.testclient import TestClient

# app.py 非包,直接把本目录加入 sys.path 后 import app
_HERE = os.path.dirname(os.path.abspath(__file__))
if _HERE not in sys.path:
    sys.path.insert(0, _HERE)


class _StubVec:
    """sentence-transformers 真实 encode 返回 torch.Tensor/numpy.ndarray(带 tolist);
    stub 用轻量对象模拟该接口,避免装 torch。所有 StubModel.encode 共用。"""
    def __init__(self, dim):
        self._data = [0.1] * dim
    def tolist(self):
        return list(self._data)


@pytest.fixture()
def client(monkeypatch):
    # 隔离 env:FORCE_MODEL 默认空(auto 路由)
    monkeypatch.delenv("EMBED_MODEL", raising=False)
    # 用 import-time reload 保证 env 生效;app.py 模块级读 FORCE_MODEL
    import importlib
    import app as appmod
    importlib.reload(appmod)
    # stub 模型加载:返回固定 dim + encode
    class StubModel:
        def __init__(self, dim):
            self._dim = dim

        def encode(self, text, normalize_embeddings=True):
            # 返回固定向量,长度=dim(带 tolist 接口,模拟真实 ndarray)
            return _StubVec(self._dim)

        def get_sentence_embedding_dimension(self):
            return self._dim

    def fake_load(short_name):
        dims = {"qwen": 2560, "minilm": 384}
        if short_name not in dims:
            raise ValueError(f"unknown model {short_name}")
        m = StubModel(dims[short_name])
        appmod._models[short_name] = m
        appmod._dims[short_name] = dims[short_name]
        return m

    monkeypatch.setattr(appmod, "_load_model", fake_load)
    with TestClient(appmod.app) as c:
        c._appmod = appmod  # type: ignore[attr-defined]
        yield c


def test_embed_qwen_dim_2560(client):
    """显式 model=qwen → 2560 维。"""
    r = client.post("/embed", json={"text": "你好世界", "model": "qwen"})
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["model"] == "qwen"
    assert body["dim"] == 2560
    assert len(body["vector"]) == 2560


def test_embed_minilm_dim_384(client):
    """显式 model=minilm → 384 维。"""
    r = client.post("/embed", json={"text": "hello world", "model": "minilm"})
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["model"] == "minilm"
    assert body["dim"] == 384
    assert len(body["vector"]) == 384


def test_embed_auto_chinese_routes_qwen(client):
    """model=auto + 中文 → qwen(2560 维)。验证 _route_model 语言路由。"""
    r = client.post("/embed", json={"text": "这是一段中文提示词", "model": "auto"})
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["model"] == "qwen"
    assert body["dim"] == 2560


def test_embed_auto_english_routes_minilm(client):
    """model=auto + 英文 → minilm(384 维)。"""
    r = client.post("/embed", json={"text": "this is an english prompt", "model": "auto"})
    assert r.status_code == 200, r.text
    body = r.json()
    assert body["model"] == "minilm"
    assert body["dim"] == 384


def test_model_priority_request_over_force(client, monkeypatch):
    """ADR-23 §2.2:请求 model 优先 > 服务端 FORCE_MODEL > 语言路由。
    显式传 model=minilm,即使 FORCE_MODEL=qwen 也应走 minilm(请求优先)。"""
    import importlib
    import app as appmod
    monkeypatch.setenv("EMBED_MODEL", "qwen")
    importlib.reload(appmod)
    assert appmod.FORCE_MODEL == "qwen"

    class StubModel:
        def __init__(self, dim):
            self._dim = dim

        def encode(self, text, normalize_embeddings=True):
            return _StubVec(self._dim)

        def get_sentence_embedding_dimension(self):
            return self._dim

    def fake_load(short_name):
        dims = {"qwen": 2560, "minilm": 384}
        m = StubModel(dims[short_name])
        appmod._models[short_name] = m
        appmod._dims[short_name] = dims[short_name]
        return m

    monkeypatch.setattr(appmod, "_load_model", fake_load)
    # 显式 minilm:embed() 中 `short = req.model if req.model in MODELS else _route_model(...)`
    # req.model="minilm" in MODELS → short=minilm,不走路由/FORCE_MODEL
    r = client.post("/embed", json={"text": "english text", "model": "minilm"})
    assert r.status_code == 200, r.text
    assert r.json()["model"] == "minilm"
    assert r.json()["dim"] == 384


def test_force_model_applies_on_auto(client, monkeypatch):
    """FORCE_MODEL=qwen + model=auto → qwen(auto 触发 _route_model,走 FORCE_MODEL)。"""
    import importlib
    import app as appmod
    monkeypatch.setenv("EMBED_MODEL", "qwen")
    importlib.reload(appmod)

    class StubModel:
        def __init__(self, dim):
            self._dim = dim

        def encode(self, text, normalize_embeddings=True):
            return _StubVec(self._dim)

        def get_sentence_embedding_dimension(self):
            return self._dim

    def fake_load(short_name):
        dims = {"qwen": 2560, "minilm": 384}
        m = StubModel(dims[short_name])
        appmod._models[short_name] = m
        appmod._dims[short_name] = dims[short_name]
        return m

    monkeypatch.setattr(appmod, "_load_model", fake_load)
    # auto + 英文,但 FORCE_MODEL=qwen → _route_model 返回 qwen
    r = client.post("/embed", json={"text": "english text here", "model": "auto"})
    assert r.status_code == 200, r.text
    assert r.json()["model"] == "qwen"
    assert r.json()["dim"] == 2560


def test_healthz(client):
    r = client.get("/healthz")
    assert r.status_code == 200
    assert r.json()["status"] == "ok"


def test_models_endpoint(client):
    r = client.get("/models")
    assert r.status_code == 200
    body = r.json()
    assert "qwen" in body["models"]
    assert "minilm" in body["models"]
