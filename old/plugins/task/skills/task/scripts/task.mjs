#!/usr/bin/env node

import fs from "fs";
import path from "path";
import { execSync } from "child_process";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function resolveRoot() {
  if (process.env.CLAUDE_PROJECT_DIR) {
    return process.env.CLAUDE_PROJECT_DIR;
  }
  try {
    return execSync("git rev-parse --show-toplevel", {
      cwd: __dirname,
      encoding: "utf-8",
      stdio: ["pipe", "pipe", "pipe"],
    }).trim();
  } catch {
    // ignore
  }
  console.error(
    "Error: Cannot determine project root.\n" +
      "Set CLAUDE_PROJECT_DIR or run from within a git repository."
  );
  process.exit(1);
}

// Lazy-init: defer root resolution so `help` works without a project context
let ROOT, TASKS_DIR, ITEMS_DIR, PHASES_DIR, LOOP_DIR, LOOP_STATE_FILE, LOOP_LOG_FILE, TASK_CMD;

function initPaths() {
  if (ROOT) return;
  ROOT = resolveRoot();
  TASKS_DIR = path.join(ROOT, "tasks");
  ITEMS_DIR = path.join(TASKS_DIR, "items");
  PHASES_DIR = path.join(TASKS_DIR, "phases");
  LOOP_DIR = path.join(TASKS_DIR, "loop");
  LOOP_STATE_FILE = path.join(LOOP_DIR, "state.json");
  LOOP_LOG_FILE = path.join(LOOP_DIR, "history.log");
  TASK_CMD = `node ${path.relative(ROOT, path.join(__dirname, "task.mjs"))}`;
}

function localTimestamp() {
  const d = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function loopLog(entry) {
  const line = `[${localTimestamp()}] ${entry}\n`;
  fs.appendFileSync(LOOP_LOG_FILE, line);
}

// ─── Output formatting ─────────────────────────────────────────────

let OUTPUT_FORMAT = "pretty"; // pretty | json

function output(data) {
  if (OUTPUT_FORMAT === "json") {
    const clean = { ...data };
    delete clean.__pretty;
    console.log(JSON.stringify(clean, null, 2));
  } else {
    if (typeof data.__pretty === "function") {
      data.__pretty();
    } else {
      console.log(data);
    }
  }
}

function fail(message) {
  output({
    error: message,
    __pretty() {
      console.error(`\n  Error: ${message}\n`);
    },
  });
  process.exit(1);
}

// ─── Stdin reader ───────────────────────────────────────────────────

function readStdin() {
  return new Promise((resolve) => {
    if (process.stdin.isTTY) {
      resolve("");
      return;
    }
    let data = "";
    process.stdin.setEncoding("utf-8");
    process.stdin.on("data", (chunk) => (data += chunk));
    process.stdin.on("end", () => resolve(data));
  });
}

// ─── Task file parser ───────────────────────────────────────────────

function parseFrontmatter(content) {
  const match = content.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/);
  if (!match) return { meta: {}, body: content };

  const meta = {};
  for (const line of match[1].split("\n")) {
    const idx = line.indexOf(":");
    if (idx === -1) continue;
    const key = line.slice(0, idx).trim();
    let val = line.slice(idx + 1).trim();
    if (val.startsWith("[") && val.endsWith("]")) {
      val = val
        .slice(1, -1)
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean);
    }
    meta[key] = val;
  }
  return { meta, body: match[2] };
}

function serializeFrontmatter(meta, body) {
  const lines = Object.entries(meta).map(([k, v]) => {
    if (Array.isArray(v)) return `${k}: [${v.join(", ")}]`;
    return `${k}: ${v}`;
  });
  return `---\n${lines.join("\n")}\n---\n${body}`;
}

function readTask(id) {
  const file = path.join(ITEMS_DIR, `${id}.md`);
  if (!fs.existsSync(file)) return null;
  const { meta, body } = parseFrontmatter(fs.readFileSync(file, "utf-8"));
  return { ...meta, id: meta.id || id, _body: body, _file: file };
}

function writeTask(task) {
  const { _body, _file, ...meta } = task;
  fs.writeFileSync(_file, serializeFrontmatter(meta, _body));
}

function allTasks() {
  if (!fs.existsSync(ITEMS_DIR)) return [];
  return fs
    .readdirSync(ITEMS_DIR)
    .filter((f) => f.endsWith(".md"))
    .map((f) => readTask(f.replace(".md", "")))
    .filter(Boolean)
    .sort((a, b) => {
      const na = parseInt(a.id.replace(/\D/g, "")) || 0;
      const nb = parseInt(b.id.replace(/\D/g, "")) || 0;
      return na - nb;
    });
}

function nextId() {
  const tasks = allTasks();
  if (tasks.length === 0) return "TASK-001";
  const max = Math.max(
    ...tasks.map((t) => parseInt(t.id.replace(/\D/g, "")) || 0)
  );
  return `TASK-${String(max + 1).padStart(3, "0")}`;
}

function isBlocked(task, allT) {
  const deps = Array.isArray(task.depends)
    ? task.depends
    : task.depends
      ? [task.depends]
      : [];
  if (deps.length === 0) return false;
  return deps.some((depId) => {
    const dep = allT.find((t) => t.id === depId);
    return !dep || dep.status !== "done";
  });
}

function today() {
  return new Date().toISOString().split("T")[0];
}

function getDeps(taskId, allT) {
  const task = allT.find((t) => t.id === taskId);
  if (!task) return [];
  const deps = Array.isArray(task.depends)
    ? task.depends
    : task.depends
      ? [task.depends]
      : [];
  return deps;
}

function getDepTree(taskId, allT, visited = new Set()) {
  if (visited.has(taskId)) return { id: taskId, circular: true, children: [] };
  visited.add(taskId);

  const task = allT.find((t) => t.id === taskId);
  if (!task) return { id: taskId, missing: true, children: [] };

  const deps = getDeps(taskId, allT);
  return {
    id: taskId,
    title: task.title,
    status: task.status,
    children: deps.map((d) => getDepTree(d, allT, new Set(visited))),
  };
}

function getReverseDeps(taskId, allT) {
  return allT.filter((t) => {
    const deps = getDeps(t.id, allT);
    return deps.includes(taskId);
  });
}

function hasCircularDep(taskId, newDeps, allT) {
  const visited = new Set();

  function walk(id) {
    if (id === taskId) return true;
    if (visited.has(id)) return false;
    visited.add(id);
    const deps = id === taskId ? newDeps : getDeps(id, allT);
    return deps.some((d) => walk(d));
  }

  return newDeps.some((d) => walk(d));
}

// ─── Validation ─────────────────────────────────────────────────────

const VALID_STATUSES = ["pending", "in_progress", "review", "done"];
const VALID_PRIORITIES = ["critical", "high", "medium", "low"];
const STATUS_TRANSITIONS = {
  pending: ["in_progress"],
  in_progress: ["review"],
  review: ["done", "in_progress"],
  done: [],
};

function validateBody(body) {
  const errors = [];
  if (!body.includes("## Description")) {
    errors.push('Body must contain "## Description" section');
  }
  if (!body.includes("## Acceptance Criteria")) {
    errors.push('Body must contain "## Acceptance Criteria" section');
  }
  return errors;
}

// ─── Phase file helpers ─────────────────────────────────────────────

const VALID_PHASE_STATUSES = ["pending", "defining", "ready", "in_progress", "done"];

function allPhases() {
  if (!fs.existsSync(PHASES_DIR)) return [];
  return fs
    .readdirSync(PHASES_DIR)
    .filter((f) => f.match(/^phase-\d+\.md$/))
    .map((f) => {
      const num = f.match(/phase-(\d+)/)[1];
      const filePath = path.join(PHASES_DIR, f);
      const { meta, body } = parseFrontmatter(
        fs.readFileSync(filePath, "utf-8")
      );
      return {
        num,
        name: meta.name || `Phase ${num}`,
        description: meta.description || "",
        status: meta.status || "defining",
        _file: filePath,
        _meta: meta,
        _body: body,
      };
    })
    .sort((a, b) => parseInt(a.num) - parseInt(b.num));
}

function writePhase(phase) {
  const { _file, _meta, _body } = phase;
  const meta = { ..._meta, name: phase.name, description: phase.description, status: phase.status };
  fs.writeFileSync(_file, serializeFrontmatter(meta, _body));
}

// ─── Display helpers ────────────────────────────────────────────────

const RESET = "\x1b[0m";
const BOLD = "\x1b[1m";
const DIM = "\x1b[2m";
const UNDERLINE = "\x1b[4m";

const FG = {
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  magenta: "\x1b[35m",
  cyan: "\x1b[36m",
  white: "\x1b[37m",
  gray: "\x1b[90m",
};

const STATUS_ICONS = {
  pending: "○",
  in_progress: "◐",
  review: "◑",
  done: "●",
  blocked: "✗",
};

const STATUS_COLORS = {
  pending: FG.gray,
  in_progress: FG.yellow,
  review: FG.blue,
  done: FG.green,
  blocked: FG.red,
};

const PRIORITY_COLORS = {
  critical: FG.red,
  high: FG.yellow,
  medium: FG.cyan,
  low: FG.gray,
};

const PHASE_STATUS_ICONS = {
  pending: "◇",
  defining: "✎",
  ready: "○",
  in_progress: "◐",
  done: "●",
};

const PHASE_STATUS_COLORS = {
  pending: FG.gray,
  defining: FG.magenta,
  ready: FG.cyan,
  in_progress: FG.yellow,
  done: FG.green,
};

function colorStatus(s) {
  const color = STATUS_COLORS[s] || "";
  const icon = STATUS_ICONS[s] || "?";
  return `${color}${icon} ${s}${RESET}`;
}

function colorPriority(p) {
  return `${PRIORITY_COLORS[p] || ""}${p}${RESET}`;
}

function colorPhaseStatus(s) {
  const color = PHASE_STATUS_COLORS[s] || "";
  const icon = PHASE_STATUS_ICONS[s] || "?";
  return `${color}${icon} ${s}${RESET}`;
}

function colorId(id) {
  return `${FG.cyan}${id}${RESET}`;
}

function progressBar(done, total, width = 20) {
  if (total === 0) return `${FG.gray}${"░".repeat(width)} 0%${RESET}`;
  const filled = Math.round((done / total) * width);
  const pct = Math.round((done / total) * 100);
  const color = pct === 100 ? FG.green : pct >= 50 ? FG.yellow : FG.gray;
  return (
    `${color}${"█".repeat(filled)}${FG.gray}${"░".repeat(width - filled)}${RESET}` +
    ` ${pct}% (${done}/${total})`
  );
}

// ─── Markdown renderer ──────────────────────────────────────────────

function renderMarkdown(text) {
  return text.split("\n").map((line) => {
    // Headers
    const h1 = line.match(/^# (.+)$/);
    if (h1) return `\n${FG.cyan}┃${RESET} ${BOLD}${FG.cyan}${h1[1]}${RESET}`;
    const h2 = line.match(/^## (.+)$/);
    if (h2) return `\n${FG.blue}│${RESET} ${BOLD}${FG.blue}${h2[1]}${RESET}`;
    const h3 = line.match(/^### (.+)$/);
    if (h3) return `${FG.magenta}▸${RESET} ${BOLD}${FG.magenta}${h3[1]}${RESET}`;
    const h4 = line.match(/^#### (.+)$/);
    if (h4) return `${FG.yellow}▹${RESET} ${BOLD}${h4[1]}${RESET}`;

    // Horizontal rule
    if (/^---+$/.test(line)) return `${DIM}${"─".repeat(40)}${RESET}`;

    // Checklist items
    line = line.replace(/^(\s*)- \[x\] (.+)$/i, `$1${FG.green}✓${RESET} ${DIM}$2${RESET}`);
    line = line.replace(/^(\s*)- \[ \] (.+)$/, `$1${FG.gray}☐${RESET} $2`);

    // Unordered list bullets
    line = line.replace(/^(\s*)- /, `$1${DIM}•${RESET} `);

    // Inline: bold
    line = line.replace(/\*\*(.+?)\*\*/g, `${BOLD}$1${RESET}`);
    // Inline: italic
    line = line.replace(/\*(.+?)\*/g, `${DIM}$1${RESET}`);
    // Inline: code
    line = line.replace(/`([^`]+)`/g, `${FG.yellow}$1${RESET}`);

    return line;
  }).join("\n");
}

// ─── Commands ───────────────────────────────────────────────────────

const commands = {
  board() {
    const tasks = allTasks();
    const phases = allPhases();

    const active = tasks.filter((t) => t.status === "in_progress");
    const review = tasks.filter((t) => t.status === "review");
    const blocked = tasks.filter(
      (t) => t.status === "pending" && isBlocked(t, tasks)
    );
    const done = tasks.filter((t) => t.status === "done");

    output({
      summary: {
        total: tasks.length,
        done: done.length,
        active: active.length,
        review: review.length,
        blocked: blocked.length,
        pending:
          tasks.length -
          done.length -
          active.length -
          review.length -
          blocked.length,
      },
      phases: phases.map((p) => {
        const pTasks = tasks.filter((t) => t.phase === p.num);
        const pDone = pTasks.filter((t) => t.status === "done").length;
        const pInProgress = pTasks.filter((t) => t.status === "in_progress").length;
        const pReview = pTasks.filter((t) => t.status === "review").length;
        const pPending = pTasks.filter((t) => t.status === "pending").length;
        const pBlocked = pTasks.filter((t) => t.status === "pending" && isBlocked(t, tasks)).length;
        return {
          phase: p.num,
          name: p.name,
          status: p.status,
          total: pTasks.length,
          done: pDone,
          in_progress: pInProgress,
          review: pReview,
          pending: pPending - pBlocked,
          blocked: pBlocked,
        };
      }),
      active: active.map((t) => ({
        id: t.id,
        title: t.title,
        feature: t.feature,
      })),
      review: review.map((t) => ({
        id: t.id,
        title: t.title,
        feature: t.feature,
      })),
      blocked: blocked.map((t) => ({
        id: t.id,
        title: t.title,
        depends: t.depends,
      })),
      __pretty() {
        console.log(`\n${BOLD}${FG.cyan}Task Board${RESET}\n`);

        if (phases.length > 0) {
          for (const p of phases) {
            const pTasks = tasks.filter((t) => t.phase === p.num);
            const pDone = pTasks.filter((t) => t.status === "done").length;
            const pInProgress = pTasks.filter((t) => t.status === "in_progress").length;
            const pReview = pTasks.filter((t) => t.status === "review").length;
            const pPending = pTasks.filter((t) => t.status === "pending").length;
            const pBlocked = pTasks.filter((t) => t.status === "pending" && isBlocked(t, tasks)).length;
            console.log(`  ${colorPhaseStatus(p.status)} ${BOLD}Phase ${p.num}: ${p.name}${RESET}`);
            if (pTasks.length > 0) {
              console.log(`    ${progressBar(pDone, pTasks.length)}`);
              const parts = [];
              if (pDone > 0) parts.push(`${FG.green}${pDone} done${RESET}`);
              if (pInProgress > 0) parts.push(`${FG.yellow}${pInProgress} active${RESET}`);
              if (pReview > 0) parts.push(`${FG.blue}${pReview} review${RESET}`);
              if (pBlocked > 0) parts.push(`${FG.red}${pBlocked} blocked${RESET}`);
              if (pPending - pBlocked > 0) parts.push(`${FG.gray}${pPending - pBlocked} pending${RESET}`);
              if (parts.length > 0) console.log(`    ${parts.join(" · ")}`);
            }
            console.log();
          }
        } else {
          console.log(
            `  Overall: ${progressBar(done.length, tasks.length)}\n`
          );
        }

        if (active.length > 0) {
          console.log(`  ${BOLD}${FG.yellow}Active:${RESET}`);
          for (const t of active)
            console.log(
              `    ${STATUS_COLORS.in_progress}${STATUS_ICONS.in_progress}${RESET} ${colorId(t.id)}  ${DIM}${t.feature || ""}${RESET}  ${t.title}`
            );
          console.log();
        }
        if (review.length > 0) {
          console.log(`  ${BOLD}${FG.blue}Review:${RESET}`);
          for (const t of review)
            console.log(
              `    ${STATUS_COLORS.review}${STATUS_ICONS.review}${RESET} ${colorId(t.id)}  ${DIM}${t.feature || ""}${RESET}  ${t.title}`
            );
          console.log();
        }
        if (blocked.length > 0) {
          console.log(`  ${BOLD}${FG.red}Blocked:${RESET}`);
          for (const t of blocked) {
            const deps = Array.isArray(t.depends)
              ? t.depends.join(", ")
              : t.depends;
            console.log(
              `    ${STATUS_COLORS.blocked}${STATUS_ICONS.blocked}${RESET} ${colorId(t.id)}  ${t.title}  ${DIM}blocked by ${deps}${RESET}`
            );
          }
          console.log();
        }
      },
    });
  },

  list(args) {
    let tasks = allTasks();
    const all = tasks;

    if (args.phase) tasks = tasks.filter((t) => t.phase === args.phase);
    if (args.status) tasks = tasks.filter((t) => t.status === args.status);
    if (args.feature) tasks = tasks.filter((t) => t.feature === args.feature);
    if (args.available)
      tasks = tasks.filter(
        (t) => t.status === "pending" && !isBlocked(t, all)
      );

    output({
      count: tasks.length,
      tasks: tasks.map((t) => ({
        id: t.id,
        title: t.title,
        status: t.status,
        phase: t.phase,
        feature: t.feature,
        priority: t.priority,
        depends: t.depends,
        spec: t.spec,
        blocked: t.status === "pending" && isBlocked(t, all),
      })),
      __pretty() {
        if (tasks.length === 0) {
          console.log("\n  No tasks found.\n");
          return;
        }
        console.log(`\n  ${BOLD}${tasks.length} task(s)${RESET}\n`);
        console.log(
          `  ${DIM}${"ID".padEnd(12)} ${"Status".padEnd(14)} ${"Phase".padEnd(6)} ${"Feature".padEnd(20)} ${"Priority".padEnd(10)} ${"Depends".padEnd(20)} Title${RESET}`
        );
        console.log(`  ${DIM}${"─".repeat(110)}${RESET}`);
        for (const t of tasks) {
          const bl = t.status === "pending" && isBlocked(t, all);
          const statusKey = bl ? "blocked" : t.status;
          const color = STATUS_COLORS[statusKey] || "";
          const icon = STATUS_ICONS[statusKey] || "?";
          const deps = Array.isArray(t.depends) ? t.depends : t.depends ? [t.depends] : [];
          const depsStr = deps.length > 0 ? deps.join(", ") : "-";
          console.log(
            `  ${color}${icon}${RESET} ${colorId((t.id || "").padEnd(10))} ${color}${statusKey.padEnd(14)}${RESET} ${(t.phase || "-").toString().padEnd(6)} ${FG.magenta}${(t.feature || "-").padEnd(20)}${RESET} ${colorPriority(t.priority || "medium").padEnd(20)} ${DIM}${depsStr.padEnd(20)}${RESET} ${t.title || ""}`
          );
        }
        console.log();
      },
    });
  },

  show(args) {
    if (!args.id) fail("Usage: task show <task-id>");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);

    const all = allTasks();
    const files = Array.isArray(task.files) ? task.files : task.files ? [task.files] : [];

    output({
      id: task.id,
      title: task.title,
      status: task.status,
      phase: task.phase,
      feature: task.feature,
      priority: task.priority,
      depends: task.depends,
      spec: task.spec,
      files,
      created: task.created,
      started: task.started,
      completed: task.completed,
      blocked: isBlocked(task, all),
      body: task._body.trim(),
      __pretty() {
        console.log(`\n${BOLD}${colorId(task.id)}: ${BOLD}${task.title}${RESET}\n`);
        console.log(
          `  ${DIM}Status:${RESET}   ${colorStatus(task.status)}`
        );
        console.log(`  ${DIM}Phase:${RESET}    ${task.phase || "-"}`);
        console.log(`  ${DIM}Feature:${RESET}  ${FG.magenta}${task.feature || "-"}${RESET}`);
        console.log(
          `  ${DIM}Priority:${RESET} ${colorPriority(task.priority || "medium")}`
        );
        if (task.depends) {
          const deps = Array.isArray(task.depends)
            ? task.depends
            : [task.depends];
          const depStatus = deps.map((d) => {
            const dt = all.find((t) => t.id === d);
            const dStatus = dt ? dt.status : "not found";
            const dColor = STATUS_COLORS[dStatus] || FG.gray;
            return `${colorId(d)} ${dColor}(${dStatus})${RESET}`;
          });
          console.log(`  ${DIM}Depends:${RESET}  ${depStatus.join(", ")}`);
        }
        if (task.spec) console.log(`  ${DIM}Spec:${RESET}     ${task.spec}`);
        if (files.length > 0) console.log(`  ${DIM}Files:${RESET}    ${files.join(", ")}`);
        if (task.created) console.log(`  ${DIM}Created:${RESET}  ${task.created}`);
        if (task.started) console.log(`  ${DIM}Started:${RESET}  ${task.started}`);
        if (task.completed) console.log(`  ${DIM}Done:${RESET}     ${task.completed}`);
        console.log(`\n${renderMarkdown(task._body.trim())}\n`);
      },
    });
  },

  async create(args) {
    if (!args.title) fail("--title is required");
    if (!args.phase) fail("--phase is required");
    if (!args.feature) fail("--feature is required");

    const id = nextId();
    const file = path.join(ITEMS_DIR, `${id}.md`);

    // Read body from stdin if --stdin flag
    let body;
    if (args.stdin) {
      const input = await readStdin();
      if (!input.trim()) fail("No input received from stdin");
      body = "\n" + input.trimEnd() + "\n\n## Log\n\n";
    } else {
      body = `\n## Description\n\nTODO: Add description\n\n## Acceptance Criteria\n\n- [ ] TODO: Define acceptance criteria\n\n## Log\n\n`;
    }

    // Validate body has required sections
    const errors = validateBody(body);
    if (errors.length > 0) fail(errors.join(". "));

    // Validate depends exist
    const depsList = args.depends
      ? args.depends.split(",").map((s) => s.trim())
      : [];
    if (depsList.length > 0) {
      const all = allTasks();
      const missing = depsList.filter((d) => !all.find((t) => t.id === d));
      if (missing.length > 0)
        fail(`Dependency not found: ${missing.join(", ")}`);
    }

    if (args.priority && !VALID_PRIORITIES.includes(args.priority))
      fail(`Invalid priority: ${args.priority}. Valid: ${VALID_PRIORITIES.join(", ")}`);

    const meta = {
      id,
      title: args.title,
      status: "pending",
      phase: args.phase,
      feature: args.feature,
      priority: args.priority || "medium",
      depends: depsList,
      spec: args.spec || "",
      files: [],
      created: today(),
      started: "",
      completed: "",
    };

    fs.writeFileSync(file, serializeFrontmatter(meta, body));

    output({
      created: id,
      title: args.title,
      file: path.relative(ROOT, file),
      __pretty() {
        console.log(
          `\n  ${STATUS_COLORS.pending}${STATUS_ICONS.pending}${RESET} Created ${BOLD}${colorId(id)}${RESET}: ${args.title}`
        );
        console.log(`  ${DIM}File: ${path.relative(ROOT, file)}${RESET}\n`);
      },
    });
  },

  start(args) {
    if (!args.id) fail("Usage: task start <task-id>");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);
    if (task.status !== "pending")
      fail(
        `Cannot start: ${args.id} is '${task.status}'. Only 'pending' tasks can be started.`
      );

    const all = allTasks();
    if (isBlocked(task, all)) {
      const deps = Array.isArray(task.depends)
        ? task.depends
        : [task.depends];
      const pending = deps.filter((d) => {
        const dt = all.find((t) => t.id === d);
        return !dt || dt.status !== "done";
      });
      fail(`Task ${args.id} is blocked by: ${pending.join(", ")}`);
    }

    task.status = "in_progress";
    task.started = today();
    writeTask(task);

    output({
      started: task.id,
      title: task.title,
      spec: task.spec,
      __pretty() {
        console.log(
          `\n  ${STATUS_COLORS.in_progress}${STATUS_ICONS.in_progress}${RESET} Started ${BOLD}${colorId(task.id)}${RESET}: ${task.title}`
        );
        if (task.spec)
          console.log(`  ${DIM}Spec:${RESET} ${task.spec}`);
        console.log();
      },
    });
  },

  async log(args) {
    if (!args.id) fail("Usage: task log <task-id> --message '...' OR --stdin");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);

    let message;
    if (args.stdin) {
      message = await readStdin();
      if (!message.trim()) fail("No input received from stdin");
      message = message.trimEnd();
    } else if (args.message) {
      message = args.message;
    } else {
      fail("Provide --message '...' or --stdin");
    }

    const date = today();
    const author = args.author || "AI Agent";
    const entry = `\n### ${date} - ${author}\n${message}\n`;
    task._body += entry;
    writeTask(task);

    output({
      logged: task.id,
      date,
      author,
      message,
      __pretty() {
        console.log(`\n  Logged to ${BOLD}${colorId(task.id)}${RESET}:`);
        console.log(`  ${DIM}${date} - ${author}${RESET}`);
        console.log(`  ${message}\n`);
      },
    });
  },

  files(args) {
    if (!args.id) fail("Usage: task files <task-id> --add 'file1,file2'");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);

    const current = Array.isArray(task.files)
      ? task.files
      : task.files
        ? [task.files]
        : [];

    if (args.add) {
      const newFiles = args.add.split(",").map((s) => s.trim());
      const merged = [...new Set([...current, ...newFiles])];
      task.files = merged;
      writeTask(task);

      output({
        id: task.id,
        files: merged,
        added: newFiles,
        __pretty() {
          console.log(`\n  Files for ${BOLD}${colorId(task.id)}${RESET}:`);
          for (const f of merged) console.log(`    ${f}`);
          console.log();
        },
      });
    } else {
      output({
        id: task.id,
        files: current,
        __pretty() {
          if (current.length === 0) {
            console.log(`\n  No files for ${colorId(task.id)}\n`);
          } else {
            console.log(`\n  Files for ${BOLD}${colorId(task.id)}${RESET}:`);
            for (const f of current) console.log(`    ${f}`);
            console.log();
          }
        },
      });
    }
  },

  deps(args) {
    if (!args.id) fail("Usage: task deps <task-id> [--reverse]");
    const all = allTasks();
    const task = all.find((t) => t.id === args.id);
    if (!task) fail(`Task not found: ${args.id}`);

    if (args.reverse) {
      const rdeps = getReverseDeps(args.id, all);
      output({
        id: args.id,
        dependents: rdeps.map((t) => ({ id: t.id, title: t.title, status: t.status })),
        __pretty() {
          if (rdeps.length === 0) {
            console.log(`\n  No tasks depend on ${BOLD}${colorId(args.id)}${RESET}\n`);
            return;
          }
          console.log(`\n  Tasks depending on ${BOLD}${colorId(args.id)}${RESET}:\n`);
          for (const t of rdeps) {
            const color = STATUS_COLORS[t.status] || "";
            console.log(`    ${color}${STATUS_ICONS[t.status] || "?"}${RESET} ${colorId(t.id)}: ${t.title} ${color}(${t.status})${RESET}`);
          }
          console.log();
        },
      });
      return;
    }

    const tree = getDepTree(args.id, all);

    function flattenTree(node, depth = 0) {
      const items = [];
      for (const child of node.children) {
        items.push({
          id: child.id,
          title: child.title || "(missing)",
          status: child.status || "missing",
          depth,
          circular: !!child.circular,
          missing: !!child.missing,
        });
        if (!child.circular && !child.missing) {
          items.push(...flattenTree(child, depth + 1));
        }
      }
      return items;
    }

    const flat = flattenTree(tree);

    output({
      id: args.id,
      dependencies: flat.map((f) => ({
        id: f.id,
        title: f.title,
        status: f.status,
        depth: f.depth,
        circular: f.circular,
      })),
      __pretty() {
        if (flat.length === 0) {
          console.log(`\n  ${BOLD}${colorId(args.id)}${RESET} has no dependencies\n`);
          return;
        }
        console.log(`\n  Dependencies of ${BOLD}${colorId(args.id)}${RESET}:\n`);

        function printNode(node, indent = "  ") {
          for (const child of node.children) {
            if (child.circular) {
              console.log(`${indent}${FG.red}↻ ${child.id} (circular!)${RESET}`);
            } else if (child.missing) {
              console.log(`${indent}${FG.red}? ${child.id} (not found)${RESET}`);
            } else {
              const color = STATUS_COLORS[child.status] || "";
              const icon = child.status === "done" ? "✓" : STATUS_ICONS[child.status] || "?";
              const label = `${colorId(child.id)}: ${child.title} ${color}(${child.status})${RESET}`;
              console.log(`${indent}${color}${icon}${RESET} ${label}`);
              if (child.children.length > 0) printNode(child, indent + "  ");
            }
          }
        }

        printNode(tree);
        console.log();
      },
    });
  },

  done(args) {
    if (!args.id) fail("Usage: task done <task-id>");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);
    if (task.status !== "in_progress")
      fail(
        `Cannot mark done: ${args.id} is '${task.status}'. Only 'in_progress' tasks can be marked done.`
      );

    task.status = "review";
    task.completed = today();
    writeTask(task);

    output({
      review: task.id,
      title: task.title,
      __pretty() {
        console.log(
          `\n  ${STATUS_COLORS.review}${STATUS_ICONS.review}${RESET} ${BOLD}${colorId(task.id)}${RESET} ready for review: ${task.title}\n`
        );
      },
    });
  },

  async approve(args) {
    if (!args.id) fail("Usage: task approve <task-id>");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);
    if (task.status !== "review")
      fail(
        `Cannot approve: ${args.id} is '${task.status}'. Only 'review' tasks can be approved.`
      );

    task.status = "done";

    let message = "";
    if (args.stdin) {
      message = (await readStdin()).trimEnd();
    } else if (args.message) {
      message = args.message;
    }

    if (message) {
      task._body += `\n### ${today()} - Developer\nApproved. ${message}\n`;
    } else {
      task._body += `\n### ${today()} - Developer\nApproved.\n`;
    }
    writeTask(task);

    output({
      approved: task.id,
      title: task.title,
      __pretty() {
        console.log(
          `\n  ${STATUS_COLORS.done}${STATUS_ICONS.done}${RESET} Approved ${BOLD}${colorId(task.id)}${RESET}: ${task.title}\n`
        );
      },
    });
  },

  async reject(args) {
    if (!args.id) fail("Usage: task reject <task-id> --message '...' OR --stdin");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);
    if (task.status !== "review")
      fail(
        `Cannot reject: ${args.id} is '${task.status}'. Only 'review' tasks can be rejected.`
      );

    let message;
    if (args.stdin) {
      message = (await readStdin()).trimEnd();
      if (!message) fail("No input received from stdin");
    } else if (args.message) {
      message = args.message;
    } else {
      fail("Provide --message '...' or --stdin for rejection reason");
    }

    task.status = "in_progress";
    task._body += `\n### ${today()} - Developer\nRejected: ${message}\n`;
    writeTask(task);

    output({
      rejected: task.id,
      title: task.title,
      reason: message,
      __pretty() {
        console.log(
          `\n  ${FG.red}↩${RESET} Rejected ${BOLD}${colorId(task.id)}${RESET}: ${message}\n`
        );
      },
    });
  },

  next() {
    const all = allTasks();
    const available = all.filter(
      (t) => t.status === "pending" && !isBlocked(t, all)
    );

    const priorityOrder = { critical: 0, high: 1, medium: 2, low: 3 };
    available.sort(
      (a, b) =>
        (priorityOrder[a.priority] ?? 2) - (priorityOrder[b.priority] ?? 2)
    );

    const task = available[0];
    if (!task) {
      output({
        next: null,
        message: "No available tasks",
        __pretty() {
          console.log("\n  No available tasks.\n");
        },
      });
      return;
    }

    output({
      next: {
        id: task.id,
        title: task.title,
        feature: task.feature,
        phase: task.phase,
        priority: task.priority,
        spec: task.spec,
        depends: task.depends,
      },
      __pretty() {
        console.log(`\n  ${BOLD}Next: ${colorId(task.id)}${RESET}`);
        console.log(`  ${DIM}Title:${RESET}    ${task.title}`);
        console.log(`  ${DIM}Feature:${RESET}  ${FG.magenta}${task.feature || "-"}${RESET}`);
        console.log(`  ${DIM}Phase:${RESET}    ${task.phase || "-"}`);
        console.log(
          `  ${DIM}Priority:${RESET} ${colorPriority(task.priority || "medium")}`
        );
        if (task.spec) console.log(`  ${DIM}Spec:${RESET}     ${task.spec}`);
        if (task.depends) {
          const deps = Array.isArray(task.depends)
            ? task.depends.join(", ")
            : task.depends;
          console.log(`  ${DIM}Depends:${RESET}  ${deps} ${FG.green}(all done ✓)${RESET}`);
        }
        console.log();
      },
    });
  },

  progress() {
    const tasks = allTasks();
    const phases = allPhases();

    const result = {
      overall: {
        total: tasks.length,
        done: tasks.filter((t) => t.status === "done").length,
      },
      phases: phases.map((p) => {
        const pTasks = tasks.filter((t) => t.phase === p.num);
        return {
          phase: p.num,
          name: p.name,
          total: pTasks.length,
          done: pTasks.filter((t) => t.status === "done").length,
          in_progress: pTasks.filter((t) => t.status === "in_progress").length,
          review: pTasks.filter((t) => t.status === "review").length,
          pending: pTasks.filter((t) => t.status === "pending").length,
        };
      }),
      __pretty() {
        console.log(`\n${BOLD}${FG.cyan}  Progress${RESET}\n`);
        console.log(
          `  Overall: ${progressBar(result.overall.done, result.overall.total)}\n`
        );
        for (const p of result.phases) {
          console.log(`  ${BOLD}Phase ${p.phase}: ${p.name}${RESET}`);
          console.log(`  ${progressBar(p.done, p.total)}`);
          const parts = [];
          if (p.done > 0) parts.push(`${FG.green}${p.done} done${RESET}`);
          if (p.in_progress > 0) parts.push(`${FG.yellow}${p.in_progress} active${RESET}`);
          if (p.review > 0) parts.push(`${FG.blue}${p.review} review${RESET}`);
          if (p.pending > 0) parts.push(`${FG.gray}${p.pending} pending${RESET}`);
          if (parts.length > 0) console.log(`  ${parts.join(" · ")}`);
          console.log();
        }
      },
    };

    output(result);
  },

  edit(args) {
    if (!args.id) fail("Usage: task edit <task-id> --field value");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);

    if (args.status)
      fail(
        `Cannot edit status directly. Use: start, done, approve, reject commands.`
      );

    if (args.title) task.title = args.title;
    if (args.phase) task.phase = args.phase;
    if (args.feature) task.feature = args.feature;
    if (args.priority) {
      if (!VALID_PRIORITIES.includes(args.priority))
        fail(
          `Invalid priority: ${args.priority}. Valid: ${VALID_PRIORITIES.join(", ")}`
        );
      task.priority = args.priority;
    }
    if (args.depends) {
      const newDeps = args.depends.split(",").map((s) => s.trim());
      const all = allTasks();
      const missing = newDeps.filter((d) => !all.find((t) => t.id === d));
      if (missing.length > 0) fail(`Dependency not found: ${missing.join(", ")}`);
      if (hasCircularDep(args.id, newDeps, all))
        fail(`Circular dependency detected. ${args.id} cannot depend on ${newDeps.join(", ")}`);
      task.depends = newDeps;
    }
    if (args.spec) task.spec = args.spec;
    writeTask(task);

    output({
      updated: task.id,
      __pretty() {
        console.log(`\n  Updated ${BOLD}${colorId(task.id)}${RESET}\n`);
      },
    });
  },

  async ["update-body"](args) {
    if (!args.id) fail("Usage: task update-body <task-id> --stdin");
    const task = readTask(args.id);
    if (!task) fail(`Task not found: ${args.id}`);

    if (!args.stdin) fail("--stdin is required for update-body");
    const input = await readStdin();
    if (!input.trim()) fail("No input received from stdin");

    // Preserve log section
    const logIdx = task._body.indexOf("## Log");
    const logSection = logIdx !== -1 ? task._body.slice(logIdx) : "## Log\n\n";

    const newBody = "\n" + input.trimEnd() + "\n\n" + logSection;
    const errors = validateBody(newBody);
    if (errors.length > 0) fail(errors.join(". "));

    task._body = newBody;
    writeTask(task);

    output({
      updated: task.id,
      __pretty() {
        console.log(`\n  Updated body of ${BOLD}${task.id}${RESET}\n`);
      },
    });
  },

  delete(args) {
    if (!args.id) fail("Usage: task delete <task-id>");
    const file = path.join(ITEMS_DIR, `${args.id}.md`);
    if (!fs.existsSync(file)) fail(`Task not found: ${args.id}`);

    // Check if other tasks depend on this
    const all = allTasks();
    const dependents = all.filter((t) => {
      const deps = Array.isArray(t.depends)
        ? t.depends
        : t.depends
          ? [t.depends]
          : [];
      return deps.includes(args.id);
    });
    if (dependents.length > 0)
      fail(
        `Cannot delete: ${dependents.map((t) => t.id).join(", ")} depend on ${args.id}`
      );

    fs.unlinkSync(file);

    output({
      deleted: args.id,
      __pretty() {
        console.log(`\n  Deleted ${args.id}\n`);
      },
    });
  },

  phase(args) {
    const phases = allPhases();

    // List all phases
    if (!args.id) {
      const tasks = allTasks();
      output({
        phases: phases.map((p) => {
          const pTasks = tasks.filter((t) => t.phase === p.num);
          const pDone = pTasks.filter((t) => t.status === "done").length;
          return {
            phase: p.num,
            name: p.name,
            status: p.status,
            description: p.description,
            tasks: pTasks.length,
            done: pDone,
          };
        }),
        __pretty() {
          console.log(`\n${BOLD}${FG.cyan}Phases${RESET}\n`);
          for (const p of phases) {
            const pTasks = tasks.filter((t) => t.phase === p.num);
            const pDone = pTasks.filter((t) => t.status === "done").length;
            console.log(`  ${colorPhaseStatus(p.status)} ${BOLD}Phase ${p.num}: ${p.name}${RESET}`);
            console.log(`    ${DIM}${p.description}${RESET}`);
            if (pTasks.length > 0) console.log(`    ${progressBar(pDone, pTasks.length)}`);
            console.log();
          }
        },
      });
      return;
    }

    // Update phase fields
    const phase = phases.find((p) => p.num === args.id);
    if (!phase) fail(`Phase not found: ${args.id}`);

    const hasUpdates = args.status || args.name || args.description;

    if (hasUpdates) {
      if (args.status) {
        if (!VALID_PHASE_STATUSES.includes(args.status))
          fail(`Invalid status: ${args.status}. Valid: ${VALID_PHASE_STATUSES.join(", ")}`);
        phase.status = args.status;
      }
      if (args.name) phase.name = args.name;
      if (args.description) phase.description = args.description;
      writePhase(phase);

      const updated = [];
      if (args.status) updated.push(`status → ${args.status}`);
      if (args.name) updated.push(`name → ${args.name}`);
      if (args.description) updated.push(`description → ${args.description}`);

      output({
        updated: `phase-${phase.num}`,
        status: phase.status,
        name: phase.name,
        description: phase.description,
        __pretty() {
          console.log(`\n  Phase ${phase.num} updated: ${updated.join(", ")}\n`);
        },
      });
    } else {
      // Show phase detail
      const allT = allTasks();
      const tasks = allT.filter((t) => t.phase === phase.num);
      output({
        phase: phase.num,
        name: phase.name,
        status: phase.status,
        description: phase.description,
        body: phase._body.trim(),
        tasks: tasks.map((t) => {
          const deps = Array.isArray(t.depends) ? t.depends : t.depends ? [t.depends] : [];
          return { id: t.id, title: t.title, status: t.status, depends: deps };
        }),
        __pretty() {
          console.log(`\n${BOLD}${FG.cyan}Phase ${phase.num}: ${phase.name}${RESET} ${colorPhaseStatus(phase.status)}\n`);
          console.log(`  ${DIM}${phase.description}${RESET}\n`);
          if (phase._body.trim()) {
            console.log(`${renderMarkdown(phase._body.trim())}\n`);
          }
          if (tasks.length > 0) {
            console.log(`${BOLD}  Tasks:${RESET}`);
            for (const t of tasks) {
              const bl = t.status === "pending" && isBlocked(t, allT);
              const statusKey = bl ? "blocked" : t.status;
              const color = STATUS_COLORS[statusKey] || "";
              const icon = STATUS_ICONS[statusKey] || "?";
              const deps = Array.isArray(t.depends) ? t.depends : t.depends ? [t.depends] : [];
              const depStr = deps.length > 0 ? ` ${DIM}← ${deps.join(", ")}${RESET}` : "";
              console.log(`  ${color}${icon}${RESET} ${colorId(t.id)}: ${t.title} ${color}(${statusKey})${RESET}${depStr}`);
            }
          } else {
            console.log(`  No tasks yet.`);
          }
          console.log();
        },
      });
    }
  },

  async ["phase-log"](args) {
    if (!args.id) fail("Usage: task phase-log <phase-num> --message '...' OR --stdin");
    const phases = allPhases();
    const phase = phases.find((p) => p.num === args.id);
    if (!phase) fail(`Phase not found: ${args.id}`);

    let message;
    if (args.stdin) {
      message = await readStdin();
      if (!message.trim()) fail("No input received from stdin");
      message = message.trimEnd();
    } else if (args.message) {
      message = args.message;
    } else {
      fail("Provide --message '...' or --stdin");
    }

    const date = today();
    const author = args.author || "AI Agent";
    const entry = `\n### ${date} - ${author}\n${message}\n`;
    phase._body += entry;
    writePhase(phase);

    output({
      logged: `phase-${phase.num}`,
      date,
      author,
      message,
      __pretty() {
        console.log(`\n  Logged to ${BOLD}Phase ${phase.num}${RESET}:`);
        console.log(`  ${DIM}${date} - ${author}${RESET}`);
        console.log(`  ${message}\n`);
      },
    });
  },

  async ["phase-update-body"](args) {
    if (!args.id) fail("Usage: task phase-update-body <phase-num> --stdin");
    const phases = allPhases();
    const phase = phases.find((p) => p.num === args.id);
    if (!phase) fail(`Phase not found: ${args.id}`);

    if (!args.stdin) fail("--stdin is required for phase-update-body");
    const input = await readStdin();
    if (!input.trim()) fail("No input received from stdin");

    // Preserve log entries (lines starting with ### date)
    const logIdx = phase._body.indexOf("### ");
    const logSection = logIdx !== -1 ? "\n" + phase._body.slice(logIdx) : "";

    phase._body = "\n" + input.trimEnd() + "\n" + logSection;
    writePhase(phase);

    output({
      updated: `phase-${phase.num}`,
      __pretty() {
        console.log(`\n  Updated body of ${BOLD}Phase ${phase.num}${RESET}\n`);
      },
    });
  },

  // ─── Loop commands ───────────────────────────────────────────────

  "loop-init"(args) {
    if (!fs.existsSync(LOOP_DIR)) fs.mkdirSync(LOOP_DIR, { recursive: true });

    if (fs.existsSync(LOOP_STATE_FILE)) {
      fail("Loop already initialized. Use loop-reset first.");
    }

    const phases = allPhases();
    if (phases.length === 0) fail("No phases found.");

    // Find target phase (only ready or in_progress phases are eligible)
    const RUNNABLE = new Set(["ready", "in_progress"]);
    let currentPhase;
    if (args.phase) {
      currentPhase = phases.find((p) => p.num === String(args.phase));
      if (!currentPhase) fail(`Phase ${args.phase} not found.`);
      if (!RUNNABLE.has(currentPhase.status))
        fail(`Phase ${args.phase} is '${currentPhase.status}'. Only 'ready' or 'in_progress' phases can be run. Change status first: task phase ${args.phase} --status ready`);
    } else {
      currentPhase = phases.find((p) => RUNNABLE.has(p.status));
      if (!currentPhase) fail("No runnable phases (status must be 'ready' or 'in_progress'). Set a phase to 'ready' first.");
    }

    // Auto-transition ready → in_progress
    if (currentPhase.status === "ready") {
      currentPhase.status = "in_progress";
      writePhase(currentPhase);
    }

    const maxIterations = parseInt(args.max) || 10;
    const lockTask = args.task || null;

    const state = {
      phase: currentPhase.num,
      task: lockTask,
      step: "analyze",
      iteration: 0,
      maxIterations,
      totalIterations: 0,
      status: "running",
      startedAt: today(),
      lockTask: !!lockTask,
    };

    fs.writeFileSync(LOOP_STATE_FILE, JSON.stringify(state, null, 2));
    loopLog(`INIT phase=${currentPhase.num} max=${maxIterations}${lockTask ? ` lock=${lockTask}` : ""}`);

    output({
      ...state,
      __pretty() {
        console.log(`\n  ${BOLD}Ralph Loop initialized${RESET}`);
        console.log(`  Phase: ${state.phase}`);
        console.log(`  Step: ${state.step}`);
        console.log(`  Max iterations per task: ${state.maxIterations}`);
        if (lockTask) console.log(`  Locked to task: ${lockTask}`);
        console.log();
      },
    });
  },

  "loop-status"() {
    if (!fs.existsSync(LOOP_STATE_FILE)) fail("Loop not initialized. Run loop-init first.");

    const state = JSON.parse(fs.readFileSync(LOOP_STATE_FILE, "utf-8"));
    const phases = allPhases();
    const tasks = allTasks();

    const phasesDone = phases.filter((p) => p.status === "done").length;
    const tasksDone = tasks.filter((t) => t.status === "done").length;

    const progress = {
      ...state,
      progress: {
        phases: { done: phasesDone, total: phases.length },
        tasks: { done: tasksDone, total: tasks.length },
      },
    };

    output({
      ...progress,
      __pretty() {
        console.log(`\n  ${BOLD}Ralph Loop Status${RESET}`);
        console.log(`  Status: ${state.status === "running" ? FG.green : state.status === "paused" ? FG.yellow : FG.blue}${state.status}${RESET}`);
        console.log(`  Phase: ${state.phase} | Step: ${state.step}`);
        if (state.task) console.log(`  Task: ${state.task}`);
        console.log(`  Iteration: ${state.iteration}/${state.maxIterations} (total: ${state.totalIterations})`);
        console.log(`  Phases: ${phasesDone}/${phases.length} done`);
        console.log(`  Tasks: ${tasksDone}/${tasks.length} done`);
        console.log();
      },
    });
  },

  "loop-reset"() {
    if (fs.existsSync(LOOP_DIR)) {
      const preserve = new Set(["ralph.sh", "history.log", "prompts"]);
      const files = fs.readdirSync(LOOP_DIR);
      for (const f of files) {
        if (preserve.has(f)) continue;
        const fp = path.join(LOOP_DIR, f);
        if (fs.statSync(fp).isDirectory()) continue;
        fs.unlinkSync(fp);
      }
    }
    loopLog("RESET");
    output({
      reset: true,
      __pretty() {
        console.log(`\n  ${BOLD}Loop state cleared${RESET}\n`);
      },
    });
  },

  "loop-prompt-init"() {
    const promptsDir = path.join(LOOP_DIR, "prompts");
    if (!fs.existsSync(promptsDir)) fs.mkdirSync(promptsDir, { recursive: true });

    const sep = "------- below this line is real content -------";
    const templates = {
      "all.append.md": `Custom instructions for ALL loop steps.
This content is appended to every prompt (analyze, implement, review).

Examples:
  - Always write code comments in English
  - Use conventional commits: feat(TASK-XXX): description
  - Prefer table-driven tests in Go

${sep}
`,
      "analyze.append.md": `Custom instructions for the ANALYZE step.
Appended when the loop analyzes a phase to decide next action.

Examples:
  - Skip tasks with feature "admin-ui" for now
  - Prioritize backend tasks over frontend

${sep}
`,
      "implement.append.md": `Custom instructions for the IMPLEMENT step.
Appended when the loop implements a task.

Examples:
  - Follow existing code patterns in the codebase
  - Always add table-driven tests
  - Use context.Context for all public functions
  - Run go test with -race flag

${sep}
`,
      "review.append.md": `Custom instructions for the REVIEW step.
Appended when the loop reviews implementation against acceptance criteria.

Examples:
  - Check for proper error wrapping with fmt.Errorf
  - Verify no TODO comments left in code
  - Ensure all public functions have doc comments

${sep}
`,
    };

    const created = [];
    const skipped = [];
    for (const [name, content] of Object.entries(templates)) {
      const fp = path.join(promptsDir, name);
      if (fs.existsSync(fp)) {
        skipped.push(name);
      } else {
        fs.writeFileSync(fp, content);
        created.push(name);
      }
    }

    output({
      created,
      skipped,
      dir: path.relative(ROOT, promptsDir),
      __pretty() {
        console.log(`\n  ${BOLD}Custom prompt templates${RESET}`);
        console.log(`  Dir: tasks/loop/prompts/\n`);
        for (const f of created) console.log(`  ${FG.green}+ ${f}${RESET}`);
        for (const f of skipped) console.log(`  ${FG.gray}  ${f} (already exists)${RESET}`);
        console.log(`\n  Edit these files to customize loop behavior.`);
        console.log(`  Write your instructions below the separator line.\n`);
      },
    });
  },

  "loop-log"(args) {
    if (!fs.existsSync(LOOP_LOG_FILE)) fail("No loop history yet.");
    const content = fs.readFileSync(LOOP_LOG_FILE, "utf-8").trim();
    const lines = content.split("\n");
    const tail = parseInt(args.tail) || 0;
    const display = tail > 0 ? lines.slice(-tail) : lines;

    output({
      entries: display,
      total: lines.length,
      __pretty() {
        console.log(`\n  ${BOLD}Ralph Loop History${RESET} (${lines.length} entries${tail ? `, showing last ${tail}` : ""})\n`);
        for (const line of display) {
          // Color-code by event type
          let colored = line;
          if (line.includes("INIT")) colored = `${FG.cyan}${line}${RESET}`;
          else if (line.includes("RESET")) colored = `${FG.magenta}${line}${RESET}`;
          else if (line.includes("START")) colored = `${FG.yellow}${line}${RESET}`;
          else if (line.includes("SHIP_REJECTED")) colored = `${FG.red}${line}${RESET}`;
          else if (line.includes("SHIP")) colored = `${FG.green}${line}${RESET}`;
          else if (line.includes("REVISE")) colored = `${FG.yellow}${line}${RESET}`;
          else if (line.includes("BLOCKED")) colored = `${FG.red}${line}${RESET}`;
          else if (line.includes("PHASE_COMPLETE")) colored = `${FG.green}${line}${RESET}`;
          else if (line.includes("RESUME")) colored = `${FG.blue}${line}${RESET}`;
          console.log(`  ${colored}`);
        }
        console.log();
      },
    });
  },

  "loop-prompt"() {
    if (!fs.existsSync(LOOP_STATE_FILE)) fail("Loop not initialized. Run loop-init first.");

    const state = JSON.parse(fs.readFileSync(LOOP_STATE_FILE, "utf-8"));
    if (state.status !== "running") fail(`Loop is ${state.status}. Cannot generate prompt.`);

    const phases = allPhases();
    const tasks = allTasks();
    const phase = phases.find((p) => p.num === state.phase);
    if (!phase) fail(`Phase ${state.phase} not found.`);

    // Read helper files
    const readLoopFile = (name) => {
      const fp = path.join(LOOP_DIR, name);
      return fs.existsSync(fp) ? fs.readFileSync(fp, "utf-8").trim() : "";
    };

    const humanInput = readLoopFile("human-input.md");
    const feedback = readLoopFile("feedback.md");
    const workSummary = readLoopFile("work-summary.md");

    // Collect feature specs
    const specsDir = path.join(ROOT, "docs", "features");
    let specsList = "";
    if (fs.existsSync(specsDir)) {
      specsList = fs.readdirSync(specsDir)
        .filter((d) => fs.statSync(path.join(specsDir, d)).isDirectory())
        .filter((d) => fs.existsSync(path.join(specsDir, d, "spec.md")))
        .map((d) => `  - docs/features/${d}/spec.md`)
        .join("\n");
    }

    const phaseTasks = tasks.filter((t) => t.phase === state.phase);
    const taskListStr = phaseTasks.length > 0
      ? phaseTasks.map((t) => {
          const blocked = isBlocked(t, tasks) ? " [BLOCKED]" : "";
          return `  - ${t.id}: ${t.title} (${t.status})${blocked}`;
        }).join("\n")
      : "  (no tasks)";

    let prompt = "";

    switch (state.step) {
      case "analyze": {
        prompt = `You are analyzing Phase ${phase.num}: ${phase.name}.
Description: ${phase.description}
Body:
${phase._body.trim()}

Current tasks in this phase:
${taskListStr}

Available feature specs:
${specsList}

Instructions:
1. Assess the current state of this phase
2. Check if all tasks are done, if there are available tasks to work on, or if new tasks need to be created

Write ONE of these results to tasks/loop/step-result.txt:
- "HAS_TASKS" — if there are available (pending, not blocked) tasks to work on
- "ALL_TASKS_DONE" — if all tasks in this phase are done (or there are no tasks)

IMPORTANT: Write ONLY the result keyword to tasks/loop/step-result.txt (no extra text).
IMPORTANT: Do NOT decide if the phase is complete. Do NOT create new tasks. Only check existing task statuses.
Do NOT implement any code. Only analyze and write the result file.`;
        break;
      }

      case "implement": {
        const task = state.task ? readTask(state.task) : null;
        if (!task) fail(`Task ${state.task} not found.`);

        const specFile = task.spec || "";
        const acMatch = task._body.match(/## Acceptance Criteria\n([\s\S]*?)(?=\n## |\n*$)/);
        const acceptanceCriteria = acMatch ? acMatch[1].trim() : "(none found in task body)";

        prompt = `You are implementing ${task.id}: ${task.title}
Iteration ${state.iteration + 1}/${state.maxIterations} for this task.

## Spec
${specFile ? `Read: ${specFile}` : "No spec file linked."}

## Acceptance Criteria
${acceptanceCriteria}

${feedback ? `## Previous Feedback\n${feedback}` : "## Previous Feedback\nFirst iteration — no feedback yet."}

${humanInput ? `## Human Guidance\n${humanInput}` : ""}

## Instructions
1. ${specFile ? `Read the spec file: ${specFile}` : "Review the task description above"}
2. ${feedback ? "Address the feedback from previous review" : "Plan your implementation approach"}
3. ${humanInput ? "Follow the human guidance provided above" : ""}
4. Implement code with tests
5. Run: \`go test ./... -count=1\` and \`go vet ./...\`
6. Track files: \`${TASK_CMD} files ${task.id} --add "file1,file2" -o json\`
7. Log progress: \`${TASK_CMD} log ${task.id} --stdin -o json << 'EOF'\n...\nEOF\`
8. Commit your changes using conventional commits with the task ID as scope:
   \`\`\`bash
   git add <files you changed>
   git commit -m "feat(${task.id}): <short description>"
   \`\`\`
   - Use \`feat\` for new features, \`fix\` for bug fixes, \`refactor\` for refactoring
   - You may make multiple commits for logically separate changes
   - Do NOT commit task state files (tasks/loop/*)
9. Write a summary of what you did to tasks/loop/work-summary.md
10. If blocked on something you cannot resolve:
    - Write "BLOCKED" to tasks/loop/step-result.txt
    - Write your questions/blockers to tasks/loop/feedback.md

IMPORTANT: When done implementing, do NOT write to step-result.txt — the loop will advance automatically.
IMPORTANT: Always commit your code changes before finishing. Uncommitted work is invisible to the next iteration.
State persists through FILES ONLY. You have no memory of previous iterations.`.replace(/\n3\. \n/, "\n");
        break;
      }

      case "review": {
        const task = state.task ? readTask(state.task) : null;
        if (!task) fail(`Task ${state.task} not found.`);

        const acMatch = task._body.match(/## Acceptance Criteria\n([\s\S]*?)(?=\n## |\n*$)/);
        const acceptanceCriteria = acMatch ? acMatch[1].trim() : "(none found in task body)";

        const filesList = Array.isArray(task.files) ? task.files.join(", ") : task.files || "(none tracked)";

        prompt = `You are reviewing ${task.id}: ${task.title}

Task file: tasks/items/${task.id}.md

## Acceptance Criteria
${acceptanceCriteria}

## Modified Files
${filesList}

## Work Summary
${workSummary || "(no summary provided)"}

## Instructions
1. Read the modified files listed above
2. Run: \`go test ./... -count=1\`
3. Run: \`go vet ./...\`
4. Check that changes are committed:
   Run \`git log --oneline -10\` and verify there are commits with "${task.id}" in the message.
   If code changes exist but are NOT committed, this is a REVISE — feedback: "commit your changes".
5. For EACH acceptance criterion, verify against actual code:
   - If met: check it off in the task file (change \`- [ ]\` to \`- [x]\`)
   - If NOT met: leave unchecked and note what's missing

6. After checking all criteria, update the task file tasks/items/${task.id}.md:
   Replace each verified \`- [ ]\` with \`- [x]\` in the Acceptance Criteria section.

7. Decide the result:
   If ALL criteria are checked AND tests pass AND changes are committed:
     Write "SHIP" to tasks/loop/step-result.txt
   If any criterion is NOT met OR changes are uncommitted:
     Write "REVISE" to tasks/loop/step-result.txt
     Write specific, actionable feedback to tasks/loop/feedback.md
     Be precise: which criterion failed, what's missing, what needs to change.

IMPORTANT: Write ONLY "SHIP" or "REVISE" to tasks/loop/step-result.txt (no extra text).
IMPORTANT: You MUST update the AC checkboxes in the task file before writing the result.`;
        break;
      }

      default:
        fail(`Unknown step: ${state.step}`);
    }

    // Append custom prompts
    const promptsDir = path.join(LOOP_DIR, "prompts");
    const CONTENT_SEPARATOR = /^-{3,}.*below.*-{3,}$/im;
    const loadAppend = (name) => {
      const fp = path.join(promptsDir, name);
      if (!fs.existsSync(fp)) return "";
      const raw = fs.readFileSync(fp, "utf-8");
      const sepMatch = raw.match(CONTENT_SEPARATOR);
      if (sepMatch) {
        const afterSep = raw.slice(raw.indexOf(sepMatch[0]) + sepMatch[0].length).trim();
        return afterSep;
      }
      return raw.trim();
    };
    const allAppend = loadAppend("all.append.md");
    const stepAppend = loadAppend(`${state.step}.append.md`);
    if (allAppend) prompt += `\n\n## Additional Instructions\n${allAppend}`;
    if (stepAppend) prompt += `\n\n## Additional Instructions (${state.step})\n${stepAppend}`;

    // Record step start time
    state.stepStartedAt = new Date().toISOString();
    fs.writeFileSync(LOOP_STATE_FILE, JSON.stringify(state, null, 2));
    loopLog(`START step=${state.step} phase=${state.phase}${state.task ? ` task=${state.task}` : ""}${state.step === "implement" ? ` iter=${state.iteration + 1}/${state.maxIterations}` : ""}`);

    // Output raw prompt (not through output() — this goes to stdout for piping)
    console.log(prompt);
  },

  "loop-advance"(args) {
    if (!fs.existsSync(LOOP_STATE_FILE)) fail("Loop not initialized. Run loop-init first.");

    const state = JSON.parse(fs.readFileSync(LOOP_STATE_FILE, "utf-8"));

    // Handle resume from paused
    if (args.resume) {
      const humanInputFile = path.join(LOOP_DIR, "human-input.md");
      if (state.status !== "paused") fail("Loop is not paused.");
      if (!fs.existsSync(humanInputFile)) fail("No human-input.md found. Write guidance there first.");
      state.status = "running";
      fs.writeFileSync(LOOP_STATE_FILE, JSON.stringify(state, null, 2));
      loopLog(`RESUME step=${state.step}${state.task ? ` task=${state.task}` : ""} (human-input provided)`);
      output({
        status: "running",
        resumed: true,
        step: state.step,
        __pretty() {
          console.log(`\n  ${FG.green}Resumed${RESET} — continuing step: ${state.step}\n`);
        },
      });
      return;
    }

    if (state.status !== "running") {
      output({
        status: state.status,
        reason: state.status === "complete" ? "All phases done" : "Waiting for human input",
        __pretty() {
          console.log(`\n  Loop is ${state.status}. ${state.status === "paused" ? "Write to tasks/loop/human-input.md and use --resume." : ""}\n`);
        },
      });
      return;
    }

    const resultFile = path.join(LOOP_DIR, "step-result.txt");
    const result = fs.existsSync(resultFile) ? fs.readFileSync(resultFile, "utf-8").trim() : "";

    const phases = allPhases();
    const tasks = allTasks();

    // Clear consumed files
    const clearFile = (name) => {
      const fp = path.join(LOOP_DIR, name);
      if (fs.existsSync(fp)) fs.unlinkSync(fp);
    };

    let nextStatus = "running";
    let reason = "";
    let action = "";

    switch (state.step) {
      case "analyze": {
        clearFile("step-result.txt");

        if (result === "ALL_TASKS_DONE") {
          // All tasks done — stop. User decides if phase is complete or adds more tasks.
          nextStatus = "complete";
          reason = `All tasks in phase ${state.phase} are done. Add more tasks or mark phase done manually.`;
          action = `Phase ${state.phase}: all tasks done. Waiting for user.`;
        } else if (result === "HAS_TASKS") {
          // Pick next available task in this phase
          const phaseTasks = tasks.filter((t) => t.phase === state.phase);
          const available = phaseTasks.filter(
            (t) => t.status === "pending" && !isBlocked(t, tasks)
          );
          const inProgress = phaseTasks.find((t) => t.status === "in_progress");

          if (state.lockTask) {
            const locked = readTask(state.task);
            if (!locked) fail(`Locked task ${state.task} not found.`);
            if (locked.status === "done") {
              nextStatus = "complete";
              reason = "Locked task is done";
              action = `Task ${state.task} is done.`;
            } else {
              if (locked.status === "pending") {
                locked.status = "in_progress";
                locked.started = today();
                writeTask(locked);
              }
              state.step = "implement";
              state.iteration = 0;
              action = `Starting locked task ${state.task}`;
            }
          } else {
            // Pick task: priority-first, then prefer in_progress over pending
            const priorityOrder = { critical: 0, high: 1, medium: 2, low: 3 };
            const candidates = [];
            if (inProgress) {
              candidates.push({ ...inProgress, _resuming: true });
            }
            for (const t of available) {
              candidates.push({ ...t, _resuming: false });
            }

            if (candidates.length > 0) {
              // Sort by: priority first, then prefer in_progress (resuming) over pending
              candidates.sort((a, b) => {
                const pa = priorityOrder[a.priority] ?? 2;
                const pb = priorityOrder[b.priority] ?? 2;
                if (pa !== pb) return pa - pb;
                // Same priority: prefer in_progress (already started)
                if (a._resuming && !b._resuming) return -1;
                if (!a._resuming && b._resuming) return 1;
                return 0;
              });

              const picked = candidates[0];
              if (picked._resuming) {
                state.task = picked.id;
                state.step = "implement";
                state.iteration = 0;
                action = `Resuming in-progress task ${picked.id} (${picked.priority})`;
              } else {
                const task = readTask(picked.id);
                task.status = "in_progress";
                task.started = today();
                writeTask(task);
                state.task = picked.id;
                state.step = "implement";
                state.iteration = 0;
                action = `Started task ${picked.id}: ${picked.title} (${picked.priority})`;
              }
            } else {
              // All pending tasks are blocked — check if any are in review
              const inReview = phaseTasks.find((t) => t.status === "review");
              if (inReview) {
                state.task = inReview.id;
                state.step = "review";
                state.iteration = 0;
                action = `Reviewing task ${inReview.id}`;
              } else {
                nextStatus = "paused";
                reason = "All tasks are blocked or done, but phase is not complete";
                action = "Paused — no actionable tasks";
              }
            }
          }
        } else {
          fail(`Unknown analyze result: "${result}". Expected HAS_TASKS or ALL_TASKS_DONE.`);
        }
        break;
      }

      case "implement": {
        // After implement, always go to review
        // Check for BLOCKED
        if (result === "BLOCKED") {
          clearFile("step-result.txt");
          nextStatus = "paused";
          reason = "AI is blocked. See tasks/loop/feedback.md for details.";
          action = "Paused — blocked. Write guidance to tasks/loop/human-input.md";
        } else {
          clearFile("step-result.txt");
          state.step = "review";
          state.totalIterations++;
          action = `Implementation done. Moving to review.`;
        }
        break;
      }

      case "review": {
        clearFile("step-result.txt");

        if (result === "SHIP") {
          // Validate AC checkboxes before marking done
          const task = readTask(state.task);
          if (task) {
            const acSection = task._body.match(/## Acceptance Criteria\n([\s\S]*?)(?=\n## |\n*$)/);
            if (acSection) {
              const acText = acSection[1];
              const unchecked = (acText.match(/- \[ \]/g) || []).length;
              const checked = (acText.match(/- \[x\]/gi) || []).length;
              if (unchecked > 0 && checked === 0) {
                // No AC checked at all — force REVISE
                state.iteration++;
                state.step = "implement";
                const feedbackFile = path.join(LOOP_DIR, "feedback.md");
                fs.writeFileSync(feedbackFile, `Review said SHIP but ${unchecked} acceptance criteria are still unchecked.\nYou must verify and check off each criterion in the task file before shipping.\n`);
                fs.writeFileSync(LOOP_STATE_FILE, JSON.stringify(state, null, 2));
                loopLog(`END result=SHIP_REJECTED ac_unchecked=${unchecked} → forcing REVISE`);
                output({
                  status: "running",
                  step: "implement",
                  task: state.task,
                  phase: state.phase,
                  iteration: state.iteration,
                  action: `SHIP rejected — ${unchecked} AC unchecked. Revising.`,
                  __pretty() {
                    console.log(`\n  ${FG.yellow}SHIP rejected${RESET} — ${unchecked} acceptance criteria still unchecked. Forcing revision.\n`);
                  },
                });
                return;
              }
            }
          }

          // Mark task done
          if (task && task.status === "in_progress") {
            task.status = "review";
            writeTask(task);
          }
          if (task && (task.status === "review" || task.status === "in_progress")) {
            task.status = "done";
            task.completed = today();
            writeTask(task);
          }

          clearFile("feedback.md");
          clearFile("work-summary.md");
          // Clear human-input after successful task completion
          clearFile("human-input.md");

          if (state.lockTask) {
            nextStatus = "complete";
            reason = "Locked task completed";
            action = `Task ${state.task} shipped! (locked task done)`;
          } else {
            state.task = null;
            state.step = "analyze";
            state.iteration = 0;
            action = `Task shipped! Re-analyzing phase.`;
          }
        } else if (result === "REVISE") {
          state.iteration++;
          if (state.iteration >= state.maxIterations) {
            nextStatus = "paused";
            reason = `Max iterations (${state.maxIterations}) reached for task ${state.task}`;
            action = `Paused — max iterations reached. Write guidance to tasks/loop/human-input.md`;
          } else {
            state.step = "implement";
            action = `Revising (iteration ${state.iteration + 1}/${state.maxIterations})`;
          }
        } else {
          fail(`Unknown review result: "${result}". Expected SHIP or REVISE.`);
        }
        break;
      }

      default:
        fail(`Unknown step: ${state.step}`);
    }

    // Compute step duration
    let duration = "";
    if (state.stepStartedAt) {
      const elapsed = Math.round((Date.now() - new Date(state.stepStartedAt).getTime()) / 1000);
      const mins = Math.floor(elapsed / 60);
      const secs = elapsed % 60;
      duration = mins > 0 ? `${mins}m${secs}s` : `${secs}s`;
      delete state.stepStartedAt;
    }

    state.status = nextStatus;
    fs.writeFileSync(LOOP_STATE_FILE, JSON.stringify(state, null, 2));

    // Log the advance
    const logParts = [`END result=${result || "auto"} → ${action}`];
    if (duration) logParts.push(`(${duration})`);
    if (reason) logParts.push(`reason="${reason}"`);
    loopLog(logParts.join(" "));

    output({
      status: nextStatus,
      step: state.step,
      task: state.task,
      phase: state.phase,
      iteration: state.iteration,
      action,
      duration: duration || undefined,
      reason: reason || undefined,
      __pretty() {
        const statusColor = nextStatus === "running" ? FG.green : nextStatus === "paused" ? FG.yellow : FG.blue;
        console.log(`\n  ${statusColor}${nextStatus}${RESET} — ${action}`);
        if (reason) console.log(`  Reason: ${reason}`);
        console.log();
      },
    });
  },
};

// ─── CLI argument parser ────────────────────────────────────────────

function parseArgs(argv) {
  const args = {};
  const positional = [];

  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--output" || arg === "-o") {
      OUTPUT_FORMAT = argv[++i] || "pretty";
    } else if (arg.startsWith("--output=")) {
      OUTPUT_FORMAT = arg.split("=")[1];
    } else if (arg === "--stdin") {
      args.stdin = true;
    } else if (arg.startsWith("--")) {
      const key = arg.slice(2);
      const eqIdx = key.indexOf("=");
      if (eqIdx !== -1) {
        args[key.slice(0, eqIdx)] = key.slice(eqIdx + 1);
      } else {
        const next = argv[i + 1];
        if (next && !next.startsWith("--")) {
          args[key] = next;
          i++;
        } else {
          args[key] = true;
        }
      }
    } else {
      positional.push(arg);
    }
  }

  return { args, positional };
}

// ─── Main ───────────────────────────────────────────────────────────

function usage() {
  console.log(`
${BOLD}Task Manager${RESET}

Usage: task <command> [options]

${BOLD}View:${RESET}
  board                            Dashboard overview
  list [--phase N] [--status S] [--feature F] [--available]
  show <task-id>                   Task details + body
  next                             Suggest next available task
  progress                         Progress per phase
  deps <task-id>                   Dependency tree
  deps <task-id> --reverse         Tasks that depend on this task
  phase                            List all phases with status
  phase <N>                        Show phase detail (with task dependencies)
  phase <N> --status <S>           Update phase status (pending|defining|ready|in_progress|done)
  phase <N> --name "..."           Update phase name
  phase <N> --description "..."    Update phase description
  phase-log <N> --message "..."    Add log entry to phase (or --stdin)
  phase-update-body <N> --stdin    Replace phase body content

${BOLD}Create & Edit:${RESET}
  create --title "..." --phase N --feature F [--priority P] [--depends ID,ID] [--spec path] [--stdin]
  edit <task-id> [--title] [--phase] [--feature] [--priority] [--depends] [--spec]
  update-body <task-id> --stdin    Replace Description + Acceptance Criteria
  delete <task-id>

${BOLD}Lifecycle:${RESET}
  start <task-id>                  pending → in_progress
  log <task-id> --message "..."    Add log entry (or --stdin)
  files <task-id> --add "f1,f2"    Track modified files
  done <task-id>                   in_progress → review
  approve <task-id> [--message]    review → done (or --stdin)
  reject <task-id> --message "."   review → in_progress (or --stdin)

${BOLD}Loop (Ralph):${RESET}
  loop-init [--phase N] [--max N] [--task ID]  Initialize loop
  loop-status                      Show loop state + progress
  loop-prompt                      Generate prompt for current step
  loop-advance [--resume]          Advance state machine
  loop-prompt-init                 Create custom prompt templates
  loop-log [--tail N]              View loop history log
  loop-reset                       Clear loop state (preserves history)

${BOLD}Options:${RESET}
  --output json|pretty             Output format (default: pretty)
  -o json                          Short form
  --stdin                          Read content from stdin (safe for long/special text)
`);
}

const { args, positional } = parseArgs(process.argv.slice(2));
const command = positional[0];

if (!command || command === "help" || args.help) {
  usage();
  process.exit(0);
}

initPaths();

// Ensure directories exist
if (!fs.existsSync(ITEMS_DIR)) fs.mkdirSync(ITEMS_DIR, { recursive: true });
if (!fs.existsSync(PHASES_DIR)) fs.mkdirSync(PHASES_DIR, { recursive: true });

// Route command
args.id = args.id || positional[1];

const cmdName = command;

if (commands[cmdName]) {
  const result = commands[cmdName](args);
  if (result instanceof Promise) result.catch((e) => fail(e.message));
} else {
  console.error(`Unknown command: ${command}`);
  console.error('Run "task help" for usage.');
  process.exit(1);
}
