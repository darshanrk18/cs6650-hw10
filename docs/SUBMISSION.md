# Submission checklist (course rubric)

Use this page against the official HW10 instructions. **Your grader needs the Khoury Git URL** even if you also keep a personal GitHub copy.

## Khoury Git repository (required)

| Item | Status |
|------|--------|
| Code & config on **github.khoury.northeastern.edu** (or whatever the course specifies) | **Action:** create a repo there and push — mirroring your personal remote is fine |
| URL submitted to the course | Paste `https://github.khoury.northeastern.edu/...` after push |

Example (GitHub CLI, Khoury host — replace ORG/REPO as instructed):

```bash
cd cs6650-hw10
GH_HOST=github.khoury.northeastern.edu gh repo create ORG/cs6650-hw10 --private --source=. --remote=khoury --push
# or: git remote add khoury git@github.khoury.northeastern.edu:YOUR_USER/cs6650-hw10.git
#     git push khoury main
```

Your current **`origin`** may remain **personal GitHub**; add **`khoury`** as a second remote and push both.

## Required artifacts in the repo

| Requirement | Where in this repo |
|-------------|-------------------|
| Leader–follower code | `cmd/leader-follower/main.go`, `internal/kvsrv/lf.go` |
| Leaderless code | `cmd/leaderless/main.go`, `internal/kvsrv/leaderless.go` |
| Load tester | `cmd/loadtest/main.go` |
| Dockerfiles | `Dockerfile.leader-follower`, `Dockerfile.leaderless`, `Dockerfile.loadtest` |
| Compose / infra config | `docker-compose.*.yml`, `terraform/*` |
| Unit / integration tests | `internal/kvsrv/cluster_test.go` — run `go test ./... -race -timeout 120s` |

## How it works (human explanation)

| Requirement | Where |
|-------------|--------|
| Walk-through for a friend: writes, reads, N/R/W, errors, tricky parts | **`docs/HANDOVER.md`** |
| Shorter API + quorum mapping | **`README.md`** |

You still need to **read the code** and be able to explain it in your own words and in Piazza/interviews (“all team members must understand AI-assisted code”).

## PDF report (required format)

The course asks for a **PDF**, not Word. This repo generates **`CS6650_HW10_Report.docx`**.

1. Open the `.docx` in Word / Google Docs / LibreOffice.
2. **Export / Save as PDF.**
3. Submit the PDF per the syllabus; optionally commit `CS6650_HW10_Report.pdf` to **Khoury** (if allowed).

Report must include:

- Histograms for **each** of the four write ratios: **read latency**, **write latency**, **time since last write to same key** (see `results/aws/figs/*.png`).
- **Discussion:** not just captions — **why** shapes differ (sync vs async write path, quorum read fan-out, local R=1 in leaderless).
- **Which LF profile does best at which write ratio?** (see **`results/aws/WRITEUP_SNIPPETS.txt`** — table updated there.)
- **Which style (w5r1 / w1r5 / r3w3 / leaderless) for which kind of application?** (same file, “Application fit” section.)

Regenerate Word after editing snippets:

```bash
python3 scripts/build_hw10_report_docx.py
```

## AI disclosure

Follow the **course** AI policy. README and the report header include a draft disclosure — adjust to match exactly what your instructor requires.
