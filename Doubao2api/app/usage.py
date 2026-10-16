"""app/usage.py — token 用量估算（豆包不回 token 数，给字符数近似）。"""


def approx(reply: str, prompt: str) -> dict:
    """豆包不回 token 数。这里给字符数近似，并在文档里标明是估算。"""
    return {"prompt_tokens": max(1, len(prompt) // 2),
            "completion_tokens": max(1, len(reply) // 2),
            "total_tokens": max(2, (len(prompt) + len(reply)) // 2)}
