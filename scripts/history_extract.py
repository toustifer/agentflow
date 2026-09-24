#!/usr/bin/env python3
"""
Agentflow History Decision Point Extractor.
Extracts decision points from SQLite task state machine, events, and git repository.
Zero third-party dependencies: strictly uses Python standard library.
Conforms to schemas/decision-point.schema.json and PROPOSAL_HISTORY_REPLAY.md §4.1.2.
"""

import argparse
import datetime
import hashlib
import json
import os
import re
import shutil
import sqlite3
import subprocess
import sys
import tempfile


def compute_sha256(filepath):
    """Compute SHA256 of a file."""
    h = hashlib.sha256()
    with open(filepath, "rb") as f:
        while chunk := f.read(65536):
            h.update(chunk)
    return h.hexdigest().upper()


def safe_json_loads(val, default):
    """Safely parse JSON or return default."""
    if val is None:
        return default
    if isinstance(val, (dict, list)):
        return val
    if not isinstance(val, str):
        return default
    s = val.strip()
    if not s:
        return default
    try:
        return json.loads(s)
    except Exception:
        return default


def ensure_str_list(val):
    """Normalize input into a list of strings."""
    if val is None:
        return []
    if isinstance(val, list):
        res = []
        for x in val:
            if x is not None:
                res.append(str(x))
        return res
    if isinstance(val, str):
        s = val.strip()
        if not s:
            return []
        # check if it is comma-separated or newline-separated
        if "\n" in s:
            return [line.strip() for line in s.splitlines() if line.strip()]
        if "," in s:
            return [part.strip() for part in s.split(",") if part.strip()]
        return [s]
    return [str(val)]


def get_git_commit(repo_path, ref="HEAD"):
    """Query git commit SHA if repo_path is valid."""
    if not repo_path or not os.path.isdir(repo_path):
        return ""
    try:
        out = subprocess.check_output(
            ["git", "-C", repo_path, "rev-parse", ref],
            stderr=subprocess.DEVNULL,
            text=True,
        ).strip()
        return out
    except Exception:
        return ""


def extract_decision_points(db_path, repo_path=None):
    """
    Safely extract decision points from SQLite database.
    Strictly reads from a temporary copy in tempdir to avoid modifying live DB.
    """
    if not os.path.exists(db_path):
        raise FileNotFoundError(f"Database not found: {db_path}")

    # Create temporary read copy in system temp
    temp_dir = tempfile.gettempdir()
    temp_copy_name = f"af_read_copy_{os.getpid()}_{datetime.datetime.now().strftime('%Y%m%d%H%M%S%f')}.db"
    temp_copy_path = os.path.join(temp_dir, temp_copy_name)

    try:
        shutil.copy2(db_path, temp_copy_path)
        # Open strictly read-only
        uri_path = f"file:{os.path.abspath(temp_copy_path)}?mode=ro&immutable=1"
        conn = sqlite3.connect(uri_path, uri=True)
        cursor = conn.cursor()

        # Check existing tables
        cursor.execute("SELECT name FROM sqlite_master WHERE type='table';")
        tables = set(row[0] for row in cursor.fetchall())

        dags_map = {}
        if "dags" in tables:
            try:
                cursor.execute("SELECT id, title, status, metadata, head_sha FROM dags;")
                for r in cursor.fetchall():
                    d_id, d_title, d_status, d_meta, d_head = r
                    dags_map[d_id] = {
                        "id": d_id or "",
                        "title": d_title or "",
                        "status": d_status or "in_progress",
                        "metadata": safe_json_loads(d_meta, {}),
                        "head_sha": d_head or "",
                    }
            except Exception:
                pass

        tasks = []
        if "tasks" in tables:
            try:
                cursor.execute("""
                    SELECT 
                        id, namespace_id, title, description, state, assigned_worker,
                        acceptance_criteria, output_files, dag_id, depends_on, tags,
                        priority, estimated_hours, actual_hours, worker_agent_id,
                        review_cycle, created_at, updated_at, metadata
                    FROM tasks
                    ORDER BY created_at ASC, id ASC;
                """)
                columns = [col[0] for col in cursor.description]
                for row in cursor.fetchall():
                    tasks.append(dict(zip(columns, row)))
            except Exception:
                pass

        events_by_task = {}
        if "events" in tables:
            try:
                cursor.execute("""
                    SELECT 
                        id, namespace_id, task_id, transition, from_state, to_state,
                        timestamp, actor, reason, metadata
                    FROM events
                    ORDER BY timestamp ASC, id ASC;
                """)
                columns = [col[0] for col in cursor.description]
                for row in cursor.fetchall():
                    ev = dict(zip(columns, row))
                    t_id = ev.get("task_id", "")
                    events_by_task.setdefault(t_id, []).append(ev)
            except Exception:
                pass

        conn.close()
    finally:
        if os.path.exists(temp_copy_path):
            try:
                os.remove(temp_copy_path)
            except Exception:
                pass

    if not tasks:
        return []

    # Build DAG task index for completed / pending tracking
    dag_tasks = {}
    for t in tasks:
        d_id = t.get("dag_id") or "default"
        dag_tasks.setdefault(d_id, []).append(t)

    git_head = get_git_commit(repo_path) if repo_path else ""

    decision_points = []
    for idx, t in enumerate(tasks):
        t_id = t.get("id") or f"task-{idx+1}"
        d_id = t.get("dag_id") or "default"
        meta = safe_json_loads(t.get("metadata"), {})
        if not isinstance(meta, dict):
            meta = {}

        # Round calculation
        raw_round = meta.get("review.round") or meta.get("review_round") or meta.get("round")
        try:
            round_num = int(raw_round) if raw_round is not None else int(t.get("review_cycle", 0) or 0) + 1
        except Exception:
            round_num = 1
        if round_num < 1:
            round_num = 1

        # Decision ID
        clean_date = ""
        created_at_str = t.get("created_at") or ""
        if created_at_str:
            match = re.search(r"(\d{4})-(\d{2})-(\d{2})", created_at_str)
            if match:
                clean_date = f"{match.group(1)}{match.group(2)}{match.group(3)}"
        if not clean_date:
            clean_date = datetime.datetime.now().strftime("%Y%m%d")
        decision_id = f"dp-{clean_date}-{idx+1:03d}"

        # Context Before
        dag_info = dags_map.get(d_id, {})
        dag_state = dag_info.get("status") or meta.get("dag_state") or "in_progress"

        all_in_dag = dag_tasks.get(d_id, [])
        completed_tasks = []
        pending_tasks = []
        for other in all_in_dag:
            o_id = other.get("id", "")
            if other.get("state") == "done" and other is not t:
                completed_tasks.append(o_id)
            else:
                pending_tasks.append(o_id)

        try:
            review_cycle = int(t.get("review_cycle", 0) or 0)
        except Exception:
            review_cycle = 0
        if review_cycle < 0:
            review_cycle = 0

        # Known evidence
        known_evidence = []
        raw_evidence = meta.get("evidence") or meta.get("review.evidence")
        if raw_evidence:
            known_evidence.extend(ensure_str_list(raw_evidence))
        notes = meta.get("review.notes") or meta.get("review_notes") or meta.get("note")
        if notes:
            known_evidence.append(str(notes))
        desc = t.get("description") or ""
        if not known_evidence and desc:
            # take first 200 chars as description snippet
            snippet = desc.strip().splitlines()[0] if desc.strip() else ""
            if snippet:
                known_evidence.append(snippet[:200])

        context_before = {
            "dag_state": str(dag_state),
            "completed_tasks": sorted(list(set(completed_tasks))),
            "pending_tasks": sorted(list(set(pending_tasks))),
            "review_cycle": review_cycle,
            "known_evidence": known_evidence,
        }

        # Decision
        assigned_worker = str(t.get("assigned_worker") or meta.get("worker.id") or meta.get("assigned_worker") or "")
        model_route = str(
            meta.get("route.model")
            or meta.get("runtime.model")
            or meta.get("model")
            or (meta.get("declared_route", {}).get("model") if isinstance(meta.get("declared_route"), dict) else "")
            or ""
        )
        raw_ac = safe_json_loads(t.get("acceptance_criteria"), [])
        acceptance_criteria = ensure_str_list(raw_ac)

        decision = {
            "assigned_worker": assigned_worker,
            "model_route": model_route,
            "acceptance_criteria": acceptance_criteria,
        }

        # Action Result
        task_events = events_by_task.get(t_id, [])
        transition = "submit"
        if task_events:
            transition = task_events[-1].get("transition") or "submit"
        elif meta.get("transition"):
            transition = str(meta["transition"])

        exit_code = 0
        if "exit_code" in meta:
            try:
                exit_code = int(meta["exit_code"])
            except Exception:
                exit_code = 0

        duration_seconds = 0.0
        try:
            actual_hours = float(t.get("actual_hours", 0) or 0)
            if actual_hours > 0:
                duration_seconds = round(actual_hours * 3600, 2)
            elif t.get("created_at") and t.get("updated_at"):
                t0 = datetime.datetime.fromisoformat(t["created_at"].replace("Z", "+00:00"))
                t1 = datetime.datetime.fromisoformat(t["updated_at"].replace("Z", "+00:00"))
                duration_seconds = max(0.0, round((t1 - t0).total_seconds(), 2))
        except Exception:
            duration_seconds = 0.0

        action_result = {
            "transition": str(transition),
            "exit_code": exit_code,
            "duration_seconds": duration_seconds,
        }

        # Outcome
        final_verdict = str(
            meta.get("review.verdict")
            or meta.get("review_decision")
            or ("pass" if t.get("state") == "done" else t.get("state") or "unknown")
        )
        review_rounds = max(1, round_num)
        rework_count = review_cycle
        rework_categories = ensure_str_list(meta.get("rework_categories", []))
        evidence_gaps = ensure_str_list(meta.get("evidence_gaps", []))

        outcome = {
            "final_verdict": final_verdict,
            "review_rounds": review_rounds,
            "rework_count": rework_count,
            "rework_categories": rework_categories,
            "evidence_gaps": evidence_gaps,
        }

        # Snapshot
        commit_sha = str(
            meta.get("review.commit")
            or meta.get("commit")
            or meta.get("commit_sha")
            or meta.get("git.head_sha")
            or dag_info.get("head_sha")
            or git_head
            or ""
        )
        raw_output_files = safe_json_loads(t.get("output_files"), [])
        evidence_files = ensure_str_list(raw_output_files)
        if not evidence_files and meta.get("output_files"):
            evidence_files = ensure_str_list(meta["output_files"])

        snapshot = {
            "worktree_head_commit": commit_sha,
            "evidence_files": evidence_files,
        }

        dp = {
            "decision_id": decision_id,
            "dag_id": str(d_id),
            "task_id": str(t_id),
            "round": round_num,
            "context_before": context_before,
            "decision": decision,
            "action_result": action_result,
            "outcome": outcome,
            "snapshot": snapshot,
        }
        decision_points.append(dp)

    return decision_points


def main():
    parser = argparse.ArgumentParser(description="Extract Agentflow Decision Points from SQLite")
    parser.add_argument("--db-path", default=r"C:\Users\15775\.dsh\agentflow\agentflow.db", help="Path to agentflow SQLite DB")
    parser.add_argument("--out-path", required=True, help="Destination JSON file path")
    parser.add_argument("--repo-path", default=os.getcwd(), help="Path to git repository")
    parser.add_argument("--validate", action="store_true", help="Validate output against schema validator")
    args = parser.parse_args()

    db_path = os.path.abspath(args.db_path)
    out_path = os.path.abspath(args.out_path)
    repo_path = os.path.abspath(args.repo_path)

    if not os.path.exists(db_path):
        print(f"ERROR: DB path does not exist: {db_path}", file=sys.stderr)
        sys.exit(2)

    # Record DB sha256 before extraction
    sha_before = compute_sha256(db_path)
    mtime_before = os.path.getmtime(db_path)

    dps = extract_decision_points(db_path, repo_path=repo_path)

    # Record DB sha256 after extraction
    sha_after = compute_sha256(db_path)
    mtime_after = os.path.getmtime(db_path)

    if sha_before != sha_after or mtime_before != mtime_after:
        print("CRITICAL SECURITY VIOLATION: Source DB was modified during extraction!", file=sys.stderr)
        sys.exit(3)

    # Write output JSON deterministically
    out_dir = os.path.dirname(out_path)
    if out_dir and not os.path.exists(out_dir):
        os.makedirs(out_dir, exist_ok=True)

    with open(out_path, "w", encoding="utf-8", newline="\n") as f:
        json.dump(dps, f, indent=2, ensure_ascii=False)
        f.write("\n")

    print(f"Extraction successful: {len(dps)} decision point(s) written to {out_path}")
    print(f"DB SHA256 integrity verified: {sha_before} (unchanged)")

    if args.validate:
        from validate_decision_points import validate_data
        errors = validate_data(dps)
        if errors:
            print(f"Schema validation FAILED with {len(errors)} error(s):", file=sys.stderr)
            for err in errors:
                print(f"  - {err}", file=sys.stderr)
            sys.exit(1)
        else:
            print(f"Schema validation PASSED for all {len(dps)} items.")

    sys.exit(0)


if __name__ == "__main__":
    main()
