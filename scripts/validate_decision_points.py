#!/usr/bin/env python3
"""
Zero-dependency validator for Agentflow Decision Point dataset.
Uses only Python standard library (json, sys, os, argparse).
Conforms to schemas/decision-point.schema.json and PROPOSAL_HISTORY_REPLAY.md §4.1.2.
"""

import argparse
import json
import os
import sys


def validate_decision_point(dp, idx=None):
    prefix = f"Item[{idx}] " if idx is not None else ""
    errors = []

    if not isinstance(dp, dict):
        return [f"{prefix}must be a JSON object, got {type(dp).__name__}"]

    required_top = [
        "decision_id",
        "dag_id",
        "task_id",
        "round",
        "context_before",
        "decision",
        "action_result",
        "outcome",
        "snapshot",
    ]
    for key in required_top:
        if key not in dp:
            errors.append(f"{prefix}missing required top-level field '{key}'")

    if errors:
        return errors

    # Check top-level scalar types
    if not isinstance(dp["decision_id"], str) or len(dp["decision_id"]) < 1:
        errors.append(f"{prefix}'decision_id' must be a non-empty string")
    if not isinstance(dp["dag_id"], str):
        errors.append(f"{prefix}'dag_id' must be a string")
    if not isinstance(dp["task_id"], str):
        errors.append(f"{prefix}'task_id' must be a string")
    if not isinstance(dp["round"], int) or isinstance(dp["round"], bool) or dp["round"] < 1:
        errors.append(f"{prefix}'round' must be an integer >= 1")

    # context_before
    cb = dp["context_before"]
    if not isinstance(cb, dict):
        errors.append(f"{prefix}'context_before' must be an object")
    else:
        req_cb = ["dag_state", "completed_tasks", "pending_tasks", "review_cycle", "known_evidence"]
        for k in req_cb:
            if k not in cb:
                errors.append(f"{prefix}'context_before' missing required field '{k}'")
        if "dag_state" in cb and not isinstance(cb["dag_state"], str):
            errors.append(f"{prefix}'context_before.dag_state' must be a string")
        if "completed_tasks" in cb and (not isinstance(cb["completed_tasks"], list) or not all(isinstance(x, str) for x in cb["completed_tasks"])):
            errors.append(f"{prefix}'context_before.completed_tasks' must be a list of strings")
        if "pending_tasks" in cb and (not isinstance(cb["pending_tasks"], list) or not all(isinstance(x, str) for x in cb["pending_tasks"])):
            errors.append(f"{prefix}'context_before.pending_tasks' must be a list of strings")
        if "review_cycle" in cb and (not isinstance(cb["review_cycle"], int) or isinstance(cb["review_cycle"], bool) or cb["review_cycle"] < 0):
            errors.append(f"{prefix}'context_before.review_cycle' must be an integer >= 0")
        if "known_evidence" in cb and (not isinstance(cb["known_evidence"], list) or not all(isinstance(x, str) for x in cb["known_evidence"])):
            errors.append(f"{prefix}'context_before.known_evidence' must be a list of strings")

    # decision
    dec = dp["decision"]
    if not isinstance(dec, dict):
        errors.append(f"{prefix}'decision' must be an object")
    else:
        req_dec = ["assigned_worker", "model_route", "acceptance_criteria"]
        for k in req_dec:
            if k not in dec:
                errors.append(f"{prefix}'decision' missing required field '{k}'")
        if "assigned_worker" in dec and not isinstance(dec["assigned_worker"], str):
            errors.append(f"{prefix}'decision.assigned_worker' must be a string")
        if "model_route" in dec and not isinstance(dec["model_route"], str):
            errors.append(f"{prefix}'decision.model_route' must be a string")
        if "acceptance_criteria" in dec and (not isinstance(dec["acceptance_criteria"], list) or not all(isinstance(x, str) for x in dec["acceptance_criteria"])):
            errors.append(f"{prefix}'decision.acceptance_criteria' must be a list of strings")

    # action_result
    ar = dp["action_result"]
    if not isinstance(ar, dict):
        errors.append(f"{prefix}'action_result' must be an object")
    else:
        req_ar = ["transition", "exit_code", "duration_seconds"]
        for k in req_ar:
            if k not in ar:
                errors.append(f"{prefix}'action_result' missing required field '{k}'")
        if "transition" in ar and not isinstance(ar["transition"], str):
            errors.append(f"{prefix}'action_result.transition' must be a string")
        if "exit_code" in ar and (not isinstance(ar["exit_code"], int) or isinstance(ar["exit_code"], bool)):
            errors.append(f"{prefix}'action_result.exit_code' must be an integer")
        if "duration_seconds" in ar and (not isinstance(ar["duration_seconds"], (int, float)) or isinstance(ar["duration_seconds"], bool) or ar["duration_seconds"] < 0):
            errors.append(f"{prefix}'action_result.duration_seconds' must be a number >= 0")

    # outcome
    oc = dp["outcome"]
    if not isinstance(oc, dict):
        errors.append(f"{prefix}'outcome' must be an object")
    else:
        req_oc = ["final_verdict", "review_rounds", "rework_count", "rework_categories", "evidence_gaps"]
        for k in req_oc:
            if k not in oc:
                errors.append(f"{prefix}'outcome' missing required field '{k}'")
        if "final_verdict" in oc and not isinstance(oc["final_verdict"], str):
            errors.append(f"{prefix}'outcome.final_verdict' must be a string")
        if "review_rounds" in oc and (not isinstance(oc["review_rounds"], int) or isinstance(oc["review_rounds"], bool) or oc["review_rounds"] < 0):
            errors.append(f"{prefix}'outcome.review_rounds' must be an integer >= 0")
        if "rework_count" in oc and (not isinstance(oc["rework_count"], int) or isinstance(oc["rework_count"], bool) or oc["rework_count"] < 0):
            errors.append(f"{prefix}'outcome.rework_count' must be an integer >= 0")
        if "rework_categories" in oc and (not isinstance(oc["rework_categories"], list) or not all(isinstance(x, str) for x in oc["rework_categories"])):
            errors.append(f"{prefix}'outcome.rework_categories' must be a list of strings")
        if "evidence_gaps" in oc and (not isinstance(oc["evidence_gaps"], list) or not all(isinstance(x, str) for x in oc["evidence_gaps"])):
            errors.append(f"{prefix}'outcome.evidence_gaps' must be a list of strings")

    # snapshot
    sn = dp["snapshot"]
    if not isinstance(sn, dict):
        errors.append(f"{prefix}'snapshot' must be an object")
    else:
        req_sn = ["worktree_head_commit", "evidence_files"]
        for k in req_sn:
            if k not in sn:
                errors.append(f"{prefix}'snapshot' missing required field '{k}'")
        if "worktree_head_commit" in sn and not isinstance(sn["worktree_head_commit"], str):
            errors.append(f"{prefix}'snapshot.worktree_head_commit' must be a string")
        if "evidence_files" in sn and (not isinstance(sn["evidence_files"], list) or not all(isinstance(x, str) for x in sn["evidence_files"])):
            errors.append(f"{prefix}'snapshot.evidence_files' must be a list of strings")

    return errors


def validate_data(data):
    if isinstance(data, list):
        all_errors = []
        for i, item in enumerate(data):
            errs = validate_decision_point(item, idx=i)
            all_errors.extend(errs)
        return all_errors
    elif isinstance(data, dict):
        return validate_decision_point(data)
    else:
        return [f"Root must be object or array, got {type(data).__name__}"]


def main():
    parser = argparse.ArgumentParser(description="Validate Decision Point JSON against schema")
    parser.add_argument("data_file", help="Path to JSON file to validate")
    parser.add_argument("--schema", help="Optional path to schema JSON file for reference", default="")
    args = parser.parse_args()

    if not os.path.exists(args.data_file):
        print(f"ERROR: File not found: {args.data_file}", file=sys.stderr)
        sys.exit(2)

    try:
        with open(args.data_file, "r", encoding="utf-8") as f:
            data = json.load(f)
    except Exception as e:
        print(f"ERROR: Failed to parse JSON in {args.data_file}: {e}", file=sys.stderr)
        sys.exit(2)

    errors = validate_data(data)
    if errors:
        print(f"Validation FAILED with {len(errors)} error(s):")
        for err in errors:
            print(f"  - {err}")
        sys.exit(1)
    else:
        count = len(data) if isinstance(data, list) else 1
        print(f"Validation PASSED: {count} decision point(s) verified.")
        sys.exit(0)


if __name__ == "__main__":
    main()
