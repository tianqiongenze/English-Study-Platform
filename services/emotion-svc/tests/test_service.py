import asyncio

from app.service import sense_emotion, visual_for_word


def test_negative_emotion_returns_comfort():
    r = sense_emotion("I feel sad and tired, this is so hard.")
    assert r.emotion == "negative"
    assert r.encouragement


def test_positive_emotion_returns_encouragement():
    r = sense_emotion("I'm so excited and confident today!")
    assert r.emotion == "positive"
    assert r.encouragement


def test_neutral_emotion_default():
    r = sense_emotion("The cat sat on the mat.")
    assert r.emotion == "neutral"


def test_visual_returns_accessible_url():
    r = asyncio.run(visual_for_word("Breathtaking", "a mountain view"))
    assert r.image_url.startswith("https://")
    assert "Breathtaking" in r.image_url or "Breathtaking" in r.alt
    assert r.alt


def test_visual_empty_word_defaults():
    r = asyncio.run(visual_for_word("", ""))
    assert r.image_url


def test_visual_cached_returns_same_result():
    """相同输入两次调用,第二次应命中缓存返回相同结果(esp-cache L1)。"""
    import os
    os.environ["CACHE_ENABLED"] = "true"
    r1 = asyncio.run(visual_for_word("Mountain", "sunny"))
    r2 = asyncio.run(visual_for_word("Mountain", "sunny"))
    assert r1.image_url == r2.image_url
