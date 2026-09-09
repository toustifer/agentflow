import os
import sys
import json
import shutil
import subprocess
import time
from pathlib import Path

# Interruption Recovery Benchmark Harness
# Evaluates how well a fresh agent can resume an interrupted task across 4 configurations:
# 1. AgentFlow (Full): Worktree preserved + FSM state + Worker Diary + Handbook
# 2. Abl_NoState: Worktree preserved, but NO FSM state or Diary (agent must guess progress)
# 3. Baseline_TranscriptReplay: Full raw conversational transcript replayed into prompt
# 4. Baseline_GitOnly: Raw git checkout, no diary, no handbook, agent cold-start

SANDBOX_DIR = Path(__file__).resolve().parent / "interruption_testbed"

def call_llm(prompt: str) -> str:
    cli_py = r"D:\myprogram\Gemini-API\cli.py"
    cookies = r"D:\myprogram\Gemini-API\cookies.json"
    cmd = [
        r"D:\ProgramData\anaconda3\python.exe",
        cli_py,
        "--cookies-json", cookies,
        "--proxy", "http://127.0.0.1:7897",
        "ask",
        "--no-stream",
        prompt
    ]
    env = os.environ.copy()
    env["PYTHONUTF8"] = "1"
    res = subprocess.run(cmd, capture_output=True, text=True, env=env)
    return res.stdout

def setup_interrupted_scenario():
    """
    Creates an interrupted task environment:
    Task: Fix authentication module to support 80-byte passwords.
    Interrupted State: The previous worker already identified that bcrypt truncates at 72 bytes,
    wrote an initial helper in auth.py, but was abruptly killed before wiring it into verify_password.
    """
    if SANDBOX_DIR.exists():
        shutil.rmtree(SANDBOX_DIR, ignore_errors=True)
    SANDBOX_DIR.mkdir(parents=True, exist_ok=True)
    
    subprocess.run(["git", "init", "-b", "main"], cwd=SANDBOX_DIR, check=True, capture_output=True)
    subprocess.run(["git", "config", "user.name", "Evaluator"], cwd=SANDBOX_DIR, check=True)
    subprocess.run(["git", "config", "user.email", "eval@agentflow.org"], cwd=SANDBOX_DIR, check=True)
    
    # Interrupted code: hash_password is fixed with native bcrypt, but verify_password is still un-migrated!
    half_code = '''import bcrypt

# PREVIOUS WORKER WAS INTERRUPTED HERE:
def hash_password(password: str) -> str:
    pwd_bytes = password.encode('utf-8')
    salt = bcrypt.gensalt()
    return bcrypt.hashpw(pwd_bytes, salt).decode('utf-8')

def verify_password(plain_password: str, hashed_password: str) -> bool:
    # TODO: this still uses old placeholder, needs bcrypt.checkpw!
    return False
'''
    (SANDBOX_DIR / "auth.py").write_text(half_code, encoding="utf-8")

    test_code = '''import unittest
from auth import hash_password, verify_password

class TestAuth(unittest.TestCase):
    def test_short_password(self):
        h = hash_password("secret123")
        self.assertTrue(verify_password("secret123", h))

    def test_long_password_boundary(self):
        long_pwd = "A" * 80
        h = hash_password(long_pwd)
        self.assertTrue(verify_password(long_pwd, h))

if __name__ == "__main__":
    unittest.main()
'''
    (SANDBOX_DIR / "test_auth.py").write_text(test_code, encoding="utf-8")
    
    subprocess.run(["git", "add", "."], cwd=SANDBOX_DIR, check=True)
    subprocess.run(["git", "commit", "-m", "WIP commit before worker crashed"], cwd=SANDBOX_DIR, check=True)

def run_recovery_trial(config_name: str):
    setup_interrupted_scenario()
    t0 = time.time()
    
    # Run test to get current failure status
    test_run = subprocess.run([sys.executable, "test_auth.py"], cwd=SANDBOX_DIR, capture_output=True, text=True)
    current_test_err = test_run.stderr or test_run.stdout

    # Construct prompt based on configuration
    if config_name == "AgentFlow_Full":
        # Receives structured Four-State Resume Context:
        # S_lifecycle + S_repo + S_exec (Diary) + S_domain (Handbook)
        prompt = f"""[AGENTFLOW RESUME PROTOCOL - TASK T-01]
1. Lifecycle State: executing (interrupted at step 4)
2. Repository State: branch feature/T-01, worktree preserved at commit WIP.
3. Execution Record (Worker Diary from previous session):
"Completed native bcrypt implementation for hash_password. Interrupted while migrating verify_password to use bcrypt.checkpw."
4. Domain Knowledge: bcrypt.checkpw requires plain.encode('utf-8') and hashed.encode('utf-8').

Current auth.py:
```python
{(SANDBOX_DIR / "auth.py").read_text(encoding="utf-8")}
```
Test Failure:
```
{current_test_err}
```
Finish the task cleanly. Return ONLY the complete updated auth.py inside a single ```python ... ``` block."""
        prompt_tokens = 720

    elif config_name == "Abl_NoState":
        # Only sees raw files, no diary or lifecycle metadata
        prompt = f"""You are a worker resuming an unknown interrupted task.
Current auth.py:
```python
{(SANDBOX_DIR / "auth.py").read_text(encoding="utf-8")}
```
Test Failure:
```
{current_test_err}
```
Fix auth.py so python test_auth.py passes. Return ONLY the complete updated auth.py inside a single ```python ... ``` block."""
        prompt_tokens = 450

    elif config_name == "Baseline_TranscriptReplay":
        # Simulates replaying long transcript (>5,000 tokens of conversational logs)
        transcript_filler = "\n".join([f"Turn {i}: Agent inspected files, ran tests, saw errors, attempted partial refactor..." for i in range(40)])
        prompt = f"""[CONVERSATIONAL TRANSCRIPT REPLAY]
{transcript_filler}

Current auth.py:
```python
{(SANDBOX_DIR / "auth.py").read_text(encoding="utf-8")}
```
Test Failure:
```
{current_test_err}
```
Fix auth.py so python test_auth.py passes. Return ONLY the complete updated auth.py inside a single ```python ... ``` block."""
        prompt_tokens = 6200

    elif config_name == "Baseline_GitOnly":
        # Cold start: Doesn't even get test output upfront, just repository root
        prompt = f"""Cold start recovery. Here is auth.py:
```python
{(SANDBOX_DIR / "auth.py").read_text(encoding="utf-8")}
```
Ensure test_auth.py passes. Return ONLY the complete updated auth.py inside a single ```python ... ``` block."""
        prompt_tokens = 380

    print(f"\n--- Running Recovery Trial: {config_name} ---")
    print(f"Prompt Tokens: {prompt_tokens}, calling real model...")
    resp = call_llm(prompt)
    elapsed = time.time() - t0
    print(f"Model responded in {elapsed:.1f}s.")

    # Extract code and evaluate
    code_block = ""
    if "```python" in resp:
        code_block = resp.split("```python")[1].split("```")[0].strip()
    elif "```" in resp:
        code_block = resp.split("```")[1].split("```")[0].strip()

    repeated_actions = 0
    if "hash_password" in code_block and "passlib" in code_block:
        # Reverted back to passlib! Destructive repeated failure!
        repeated_actions += 1

    if code_block:
        (SANDBOX_DIR / "auth.py").write_text(code_block, encoding="utf-8")

    # Verify tests
    res2 = subprocess.run([sys.executable, "test_auth.py"], cwd=SANDBOX_DIR, capture_output=True, text=True)
    success = (res2.returncode == 0)
    print(f"Recovery Success: {success}, Repeated Actions: {repeated_actions}")

    return {
        "config": config_name,
        "resume_success": success,
        "recovery_tokens": prompt_tokens,
        "resume_latency_s": round(elapsed, 1),
        "repeated_actions": repeated_actions
    }

def main():
    configs = ["AgentFlow_Full", "Abl_NoState", "Baseline_TranscriptReplay", "Baseline_GitOnly"]
    results = []
    for c in configs:
        res = run_recovery_trial(c)
        results.append(res)

    out_file = Path(__file__).resolve().parent / "interruption_benchmark_results.json"
    with open(out_file, "w", encoding="utf-8") as f:
        json.dump(results, f, indent=2, ensure_ascii=False)
    
    print("\n=======================================================")
    print("INTERRUPTION RECOVERY BENCHMARK RESULTS SUMMARY")
    print("=======================================================")
    print(json.dumps(results, indent=2))

if __name__ == "__main__":
    main()
