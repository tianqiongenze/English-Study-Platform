from pydantic import BaseModel


class SenseRequest(BaseModel):
    text: str = ""
    audio: str = ""
    user_id: int = 0  # 情绪事件持久化到 learning_events(0=不持久化)


class SenseResponse(BaseModel):
    emotion: str
    encouragement: str


class VisualRequest(BaseModel):
    word: str
    context: str = ""


class VisualResponse(BaseModel):
    image_url: str
    alt: str
