const KEYWORDS = new Set([
  "resource",
  "data",
  "module",
  "variable",
  "output",
  "locals",
  "provider",
  "terraform",
  "required_version",
  "required_providers",
  "backend",
  "provisioner",
  "lifecycle",
  "dynamic",
  "for_each",
  "count",
  "if",
  "else",
  "null",
  "true",
  "false"
]);

export function buildRunBundleURL(scenario, runID) {
  return `/api/runs/${encodeURIComponent(scenario)}/${encodeURIComponent(runID)}/bundle.zip`;
}

export function buildRunArtifactsURL(scenario, runID) {
  return `/api/runs/${encodeURIComponent(scenario)}/${encodeURIComponent(runID)}/artifacts.zip`;
}

export function buildSnapshotLabel(snapshot) {
  if (snapshot === "final") {
    return "Final output";
  }
  return `Iteration ${snapshot}`;
}

export function buildSnapshotOptions(iterations) {
  return ["final", ...[...iterations].reverse()];
}

export function getDefaultCompareSnapshot(selectedSnapshot, iterations) {
  if (selectedSnapshot === "final") {
    return iterations.length > 0 ? iterations[iterations.length - 1] : null;
  }
  const ordered = [...iterations].sort((a, b) => a - b);
  const idx = ordered.indexOf(selectedSnapshot);
  if (idx > 0) {
    return ordered[idx - 1];
  }
  return "final";
}

export function highlightHCL(source) {
  const lines = source.split("\n");
  const highlighted = [];
  let inBlockComment = false;
  let heredocTag = "";

  for (const line of lines) {
    if (heredocTag) {
      highlighted.push(highlightHeredocLine(line, heredocTag));
      if (line.trim() === heredocTag) {
        heredocTag = "";
      }
      continue;
    }

    if (inBlockComment) {
      const endIdx = line.indexOf("*/");
      if (endIdx >= 0) {
        highlighted.push([
          { text: line.slice(0, endIdx + 2), className: "token-comment" },
          ...tokenizeHCL(line.slice(endIdx + 2))
        ]);
        inBlockComment = false;
      } else {
        highlighted.push([{ text: line, className: "token-comment" }]);
      }
      continue;
    }

    const blockStartIdx = line.indexOf("/*");
    if (blockStartIdx >= 0 && (line.indexOf("//") === -1 || blockStartIdx < line.indexOf("//"))) {
      const endIdx = line.indexOf("*/", blockStartIdx + 2);
      if (endIdx >= 0) {
        highlighted.push([
          ...tokenizeHCL(line.slice(0, blockStartIdx)),
          { text: line.slice(blockStartIdx, endIdx + 2), className: "token-comment" },
          ...tokenizeHCL(line.slice(endIdx + 2))
        ]);
      } else {
        highlighted.push([
          ...tokenizeHCL(line.slice(0, blockStartIdx)),
          { text: line.slice(blockStartIdx), className: "token-comment" }
        ]);
        inBlockComment = true;
      }
      continue;
    }

    const heredocMatch = line.match(/<<-?([A-Za-z_][A-Za-z0-9_]*)/);
    if (heredocMatch) {
      heredocTag = heredocMatch[1];
    }

    highlighted.push(tokenizeHCL(line));
  }

  return highlighted;
}

export function buildLineDiff(beforeText, afterText) {
  const before = beforeText.split("\n");
  const after = afterText.split("\n");
  const lcs = buildLCSMatrix(before, after);
  const edits = [];

  let i = before.length;
  let j = after.length;
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && before[i - 1] === after[j - 1]) {
      edits.push({ type: "context", before: before[i - 1], after: after[j - 1] });
      i--;
      j--;
      continue;
    }
    if (j > 0 && (i === 0 || lcs[i][j - 1] >= lcs[i - 1][j])) {
      edits.push({ type: "add", before: "", after: after[j - 1] });
      j--;
      continue;
    }
    edits.push({ type: "remove", before: before[i - 1], after: "" });
    i--;
  }

  const reversed = edits.reverse();
  const rows = [];
  let beforeLine = 1;
  let afterLine = 1;
  for (const edit of reversed) {
    rows.push({
      ...edit,
      beforeLine: edit.type === "add" ? "" : beforeLine,
      afterLine: edit.type === "remove" ? "" : afterLine
    });
    if (edit.type !== "add") beforeLine++;
    if (edit.type !== "remove") afterLine++;
  }
  return rows;
}

function buildLCSMatrix(before, after) {
  const dp = Array.from({ length: before.length + 1 }, () => Array(after.length + 1).fill(0));
  for (let i = 1; i <= before.length; i++) {
    for (let j = 1; j <= after.length; j++) {
      if (before[i - 1] === after[j - 1]) {
        dp[i][j] = dp[i - 1][j - 1] + 1;
      } else {
        dp[i][j] = Math.max(dp[i - 1][j], dp[i][j - 1]);
      }
    }
  }
  return dp;
}

function tokenizeHCL(line) {
  const tokens = [];
  let i = 0;
  while (i < line.length) {
    const rest = line.slice(i);

    if (rest.startsWith("//") || rest.startsWith("#")) {
      tokens.push({ text: rest, className: "token-comment" });
      break;
    }

    if (/\s/.test(rest[0])) {
      const match = rest.match(/^\s+/);
      tokens.push({ text: match[0], className: "token-space" });
      i += match[0].length;
      continue;
    }

    if (rest.startsWith('"')) {
      const [text, consumed] = consumeString(rest);
      tokens.push(...highlightString(text));
      i += consumed;
      continue;
    }

    if (rest.startsWith("${")) {
      tokens.push({ text: "${", className: "token-interpolation" });
      i += 2;
      continue;
    }

    const heredoc = rest.match(/^<<-?[A-Za-z_][A-Za-z0-9_]*/);
    if (heredoc) {
      tokens.push({ text: heredoc[0], className: "token-heredoc" });
      i += heredoc[0].length;
      continue;
    }

    const number = rest.match(/^-?\d+(?:\.\d+)?/);
    if (number) {
      tokens.push({ text: number[0], className: "token-number" });
      i += number[0].length;
      continue;
    }

    const operator = rest.match(/^(==|!=|>=|<=|=>|\.\.\.|&&|\|\||[=+\-*/%?:[\]{}(),.<>])/);
    if (operator) {
      tokens.push({ text: operator[0], className: "token-punct" });
      i += operator[0].length;
      continue;
    }

    const ident = rest.match(/^[A-Za-z_][A-Za-z0-9_-]*/);
    if (ident) {
      const text = ident[0];
      const next = rest.slice(text.length);
      let className = "token-ident";
      if (KEYWORDS.has(text)) {
        className = text === "true" || text === "false" ? "token-boolean" : "token-keyword";
      } else if (next.startsWith("(")) {
        className = "token-function";
      } else if (/^\s*=/.test(next)) {
        className = "token-attribute";
      }
      tokens.push({ text, className });
      i += text.length;
      continue;
    }

    tokens.push({ text: rest[0], className: "token-punct" });
    i++;
  }
  return tokens;
}

function consumeString(rest) {
  let escaped = false;
  for (let idx = 1; idx < rest.length; idx++) {
    const ch = rest[idx];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (ch === "\\") {
      escaped = true;
      continue;
    }
    if (ch === '"') {
      return [rest.slice(0, idx + 1), idx + 1];
    }
  }
  return [rest, rest.length];
}

function highlightString(text) {
  const tokens = [];
  let i = 0;
  while (i < text.length) {
    const start = text.indexOf("${", i);
    if (start === -1) {
      tokens.push({ text: text.slice(i), className: "token-string" });
      break;
    }
    if (start > i) {
      tokens.push({ text: text.slice(i, start), className: "token-string" });
    }
    let depth = 1;
    let end = start + 2;
    while (end < text.length && depth > 0) {
      if (text.startsWith("${", end)) {
        depth++;
        end += 2;
        continue;
      }
      if (text[end] === "}") {
        depth--;
      }
      end++;
    }
    tokens.push({ text: text.slice(start, end), className: "token-interpolation" });
    i = end;
  }
  return tokens;
}

function highlightHeredocLine(line, heredocTag) {
  if (line.trim() === heredocTag) {
    return [{ text: line, className: "token-heredoc" }];
  }
  return [{ text: line, className: "token-string" }];
}
