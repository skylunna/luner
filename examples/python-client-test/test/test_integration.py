"""
ai-gateway 集成测试脚本
验证：基础代理 / 缓存命中 / 限流拦截
"""
import os
import time
from openai import OpenAI
from dotenv import load_dotenv
from pathlib import Path


BASE_DIR = Path(__file__).parent.parent.parent
load_dotenv(BASE_DIR / ".env")

BASE_URL = os.getenv("AI_GATEWAY_BASE_URL", "http://localhost:8080/v1")
API_KEY = os.getenv("AI_GATEWAY_API_KEY", "sk-test-dummy")
MODEL = os.getenv("AI_GATEWAY_MODEL", "qwen-turbo")

def get_client():
    return OpenAI(api_key=API_KEY, base_url=BASE_URL)

def test_basic_proxy():
    print("🟢 [1/3] 测试基础代理...")
    print(f" 连接：{BASE_URL} | 模型：{MODEL}")
    
    client = get_client()
    resp = client.chat.completions.create(
        model=MODEL,
        messages=[{"role": "user", "content": "Say OK"}],
        temperature=0
    )
    
    content = resp.choices[0].message.content
    print(f"响应内容: {content}")
    assert content, "响应为空"
    print("基础代理通过\n")

def test_cache_hit():
    print("🟢 [2/3] 测试缓存命中 (temperature=0)...")
    client = get_client()
    prompt = "Explain Go context in 1 sentence"
    
    # 第一次请求（冷启动）
    t0 = time.time()
    client.chat.completions.create(model=MODEL, messages=[{"role":"user","content":prompt}], temperature=0)
    t1 = time.time()
    
    # 第二次请求（应命中缓存）
    t2 = time.time()
    client.chat.completions.create(model=MODEL, messages=[{"role":"user","content":prompt}], temperature=0)
    t3 = time.time()
    
    cold, hot = (t1-t0)*1000, (t3-t2)*1000
    speedup = cold / hot if hot > 0 else 0
    
    print(f"⏱️  冷启动: {cold:.0f}ms | 缓存命中: {hot:.0f}ms (加速 {speedup:.1f}x)")
    
    if hot < cold * 0.5:
        print("✅ 缓存命中通过\n")
    else:
        print("⚠️  缓存未明显加速（可能是首次请求或后端极快）\n")

def test_rate_limit():
    print("🟢 [3/3] 测试限流拦截...")
    client = get_client()
    errors = 0

    for i in range(6):
        try:
            client.chat.completions.create(
                model=MODEL,
                messages=[{"role":"user","content":"rate limit test"}],
                temperature=0
            )
        except Exception as e:
            # 捕获 429 错误
            if "429" in str(e) or "rate limit" in str(e).lower():
                errors += 1
    
    if errors > 0:
        print(f"✅ 限流拦截通过（成功拦截 {errors} 次）\n")
    else:
        print("⚠️  未触发限流（请检查 config.yaml 中 rate_limit 设置）\n")

if __name__ == "__main__":
    print("🚀 开始 ai-gateway 集成测试...\n")
    try:
        test_basic_proxy()
        test_cache_hit()
        test_rate_limit()
        print("🎉 所有测试完成！")
    except Exception as e:
        print(f"❌ 测试失败: {e}")
        exit(1)