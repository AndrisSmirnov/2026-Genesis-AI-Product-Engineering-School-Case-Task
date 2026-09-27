#!/usr/bin/env python3
"""Grade one eval run from Claude Code stream-json transcripts (stdlib only).

usage: check.py <scenario_id> <run_dir>
run_dir contains turn1.jsonl [turn2.jsonl] and the workdir/ the agent used.
Prints one JSON line: {"scenario", "pass", "failed": [...], "metrics": {...}}.
"""
import glob
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
CMD_RE = re.compile(r"wikitrend\s+(doctor|resolve|analyze|report|explain)\b")
NUM_RE = re.compile(r"[-+−]?\d+(?:[.,]\d+)?")
PCT_RE = re.compile(r"([-+−]?\d+(?:[.,]\d+)?)\s*%")


def load(path):
    events = []
    if not os.path.exists(path):
        return events
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line.startswith("{"):
                try:
                    events.append(json.loads(line))
                except json.JSONDecodeError:
                    pass
    return events


def parse(events):
    t = {"commands": [], "bash": [], "writes": [], "tool_outputs": [], "answer": "", "cost": 0.0,
         "turns": 0, "duration_ms": 0, "tool_calls": 0, "skill_used": False, "cli_errors": 0,
         "input_tokens": 0, "output_tokens": 0}
    for e in events:
        if e.get("type") == "assistant":
            for c in e.get("message", {}).get("content", []):
                if c.get("type") != "tool_use":
                    continue
                t["tool_calls"] += 1
                name, inp = c.get("name"), c.get("input", {})
                if name == "Skill" and "wiki-interest-trends" in json.dumps(inp):
                    t["skill_used"] = True
                if name == "Read" and "wiki-interest-trends/SKILL.md" in inp.get("file_path", ""):
                    t["skill_used"] = True
                if name == "Bash":
                    cmd = inp.get("command", "")
                    t["bash"].append(cmd)
                    for m in CMD_RE.finditer(cmd):
                        t["commands"].append((m.group(1), cmd))
                if name in ("Write", "Edit", "MultiEdit", "NotebookEdit"):
                    t["writes"].append(inp.get("file_path", ""))
        elif e.get("type") == "user":
            for c in e.get("message", {}).get("content", []) if isinstance(e.get("message", {}).get("content"), list) else []:
                if c.get("type") == "tool_result":
                    content = c.get("content")
                    if isinstance(content, list):
                        content = " ".join(x.get("text", "") for x in content if isinstance(x, dict))
                    content = str(content)
                    t["tool_outputs"].append(content)
                    if '"status":"error"' in content.replace(" ", ""):
                        t["cli_errors"] += 1
        elif e.get("type") == "result":
            t["answer"] = e.get("result", "") or ""
            t["cost"] = e.get("total_cost_usd", 0) or 0
            t["turns"] = e.get("num_turns", 0) or 0
            t["duration_ms"] = e.get("duration_ms", 0) or 0
            u = e.get("usage", {}) or {}
            t["input_tokens"] = (u.get("input_tokens", 0) or 0) + (u.get("cache_read_input_tokens", 0) or 0) + (u.get("cache_creation_input_tokens", 0) or 0)
            t["output_tokens"] = u.get("output_tokens", 0) or 0
    return t


def num(s):
    return float(s.replace("−", "-").replace(",", "."))


def grounded(answer, outputs):
    """Every percentage in the answer must appear in some tool output (rounding allowed)."""
    known = set()
    for o in outputs:
        for m in NUM_RE.findall(o):
            try:
                known.add(abs(num(m)))
            except ValueError:
                pass
    bad = []
    for m in PCT_RE.findall(answer):
        v = abs(num(m))
        ok = any(abs(v - k) <= 0.051 or (v == int(v) and round(k) == v) or abs(v - round(k, 1)) <= 0.051 for k in known)
        if not ok:
            bad.append(m + "%")
    return bad


def last_analyze_json(outputs):
    res = None
    for o in outputs:
        for line in o.splitlines():
            line = line.strip()
            if line.startswith("{") and '"period"' in line and '"run_id"' in line:
                try:
                    res = json.loads(line)
                except json.JSONDecodeError:
                    pass
    return res


def main():
    sid, run_dir = sys.argv[1], sys.argv[2]
    sc = {s["id"]: s for s in json.load(open(os.path.join(HERE, "scenarios.json")))["scenarios"]}[sid]
    ch = sc["checks"]
    t1 = parse(load(os.path.join(run_dir, "turn1.jsonl")))
    t2 = parse(load(os.path.join(run_dir, "turn2.jsonl"))) if sc.get("followup") else None
    failed = []

    def need(cond, msg):
        if not cond:
            failed.append(msg)

    cmds1 = [c for c, _ in t1["commands"]]
    if not ch.get("negative"):
        need(t1["skill_used"] or cmds1, "skill not used")
    for c in ch.get("must_run", []):
        need(c in cmds1, f"did not run {c}")
    for c in ch.get("must_not_run", []):
        need(c not in cmds1, f"ran {c} but should not")

    all_t = [t1] + ([t2] if t2 else [])
    seen_outputs = []
    for t in all_t:
        # A follow-up answer may legitimately quote numbers from earlier turns.
        seen_outputs += t["tool_outputs"]
        code_files = [w for w in t["writes"] if re.search(r"\.(py|go|js|ts|sh|r)$", w, re.I)]
        need(not code_files, f"wrote code files {code_files}")
        diy = [b for b in t["bash"] if re.search(r"\b(python3?|node|curl|wget)\b", b) and "wikitrend" not in b]
        need(not diy, f"did its own data work: {diy[:2]}")
        bad = grounded(t["answer"], seen_outputs)
        need(not bad, f"percentages not found in tool output: {bad}")
    need(t1["answer"].strip() != "", "empty answer")

    for group in ch.get("answer_any", []):
        need(any(w.lower() in t1["answer"].lower() for w in group), f"answer lacks any of {group}")
    if ch.get("answer_has_percent"):
        need(bool(PCT_RE.search(t1["answer"])), "answer has no percentage")
    if ch.get("asks_user"):
        need("?" in t1["answer"], "does not ask the user")
    a = last_analyze_json(t1["tool_outputs"])
    if "analyze_months" in ch:
        need(a is not None and a.get("period", {}).get("months") == ch["analyze_months"], f"window is not {ch['analyze_months']} months")
    if ch.get("basket"):
        need(any("--qid" in c and "," in c.split("--qid")[1].split()[0] or ";" in c for n, c in t1["commands"] if n == "resolve"), "no basket (several topics) used")
    work = os.path.join(run_dir, "workdir")
    if ch.get("pdf"):
        pdfs = glob.glob(os.path.join(work, "wikitrend-data/runs/*/report.pdf"))
        need(bool(pdfs), "no report.pdf")
        for p in pdfs:
            pages = len(re.findall(rb"/Type\s*/Page[^s]", open(p, "rb").read()))
            need(pages == 1, f"{p} has {pages} pages")
    if "criteria" in ch:
        ok = False
        for sp in glob.glob(os.path.join(work, "wikitrend-data/runs/*/spec.json")):
            c = json.load(open(sp)).get("criteria", {})
            if all(abs(c.get(k, -1) - v) < 1e-9 for k, v in ch["criteria"].items()):
                ok = True
        need(ok, f"criteria {ch['criteria']} not in any spec")
    if t2:
        cmds2 = [c for c, _ in t2["commands"]]
        for c in ch.get("followup_must_run", []):
            need(c in cmds2, f"follow-up did not run {c}")
        for c in ch.get("followup_must_not_run", []):
            need(c not in cmds2, f"follow-up ran {c}")
        if ch.get("followup_from"):
            need(any("--from" in c for n, c in t2["commands"] if n == "analyze"), "follow-up did not use analyze --from")
        a2 = last_analyze_json(t2["tool_outputs"])
        if "followup_months" in ch:
            need(a2 is not None and a2.get("period", {}).get("months") == ch["followup_months"], "follow-up window wrong")
        if ch.get("followup_network_zero"):
            need(a2 is not None and a2.get("requests", {}).get("network") == 0, "follow-up hit the network (cache not used)")

    metrics = {k: sum(t[k] for t in all_t) for k in ("cost", "turns", "duration_ms", "tool_calls", "cli_errors", "input_tokens", "output_tokens")}
    metrics["commands"] = [c for t in all_t for c, _ in t["commands"]]
    print(json.dumps({"scenario": sid, "run": os.path.basename(run_dir), "pass": not failed, "failed": failed, "metrics": metrics}, ensure_ascii=False))


if __name__ == "__main__":
    main()
