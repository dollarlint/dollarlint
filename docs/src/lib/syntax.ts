type Language = "json" | "toml" | "yaml";

const escapeHtml = (value: string) =>
  value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");

const token = (kind: string, value: string) =>
  `<span class="syntax-${kind}">${escapeHtml(value)}</span>`;

const highlightPattern = (
  line: string,
  pattern: RegExp,
  kindForMatch: (match: string, start: number, line: string) => string,
) => {
  let output = "";
  let index = 0;

  for (const match of line.matchAll(pattern)) {
    const value = match[0];
    const start = match.index ?? 0;
    output += escapeHtml(line.slice(index, start));
    output += token(kindForMatch(value, start, line), value);
    index = start + value.length;
  }

  return output + escapeHtml(line.slice(index));
};

const highlightJsonLine = (line: string) =>
  highlightPattern(
    line,
    /"([^"\\]|\\.)*"\s*(?=:)|"([^"\\]|\\.)*"|-?\b\d+(?:\.\d+)?(?:[eE][+-]?\d+)?\b|\b(?:true|false|null)\b|[{}\[\],:]/g,
    (match, start, source) => {
      if (match.startsWith('"')) {
        if (
          source
            .slice(start + match.length)
            .trimStart()
            .startsWith(":")
        ) {
          return "key";
        }
        return "string";
      }
      if (/^-?\d/.test(match)) {
        return "number";
      }
      if (/^(true|false|null)$/.test(match)) {
        return "literal";
      }
      return "punctuation";
    },
  );

const splitComment = (line: string) => {
  const index = line.indexOf("#");
  return index === -1 ? [line, ""] : [line.slice(0, index), line.slice(index)];
};

const highlightTomlBody = (line: string) =>
  highlightPattern(
    line,
    /^\s*(?:\[\[?[^\]]+\]?\])|(?:^|\s)(?:"[^"]+"|[A-Za-z0-9_.-]+)(?=\s*=)|"([^"\\]|\\.)*"|-?\b\d+(?:\.\d+)?\b|\b(?:true|false)\b|[=\[\],]/g,
    (match) => {
      const trimmed = match.trim();
      if (trimmed.startsWith("[") && trimmed.endsWith("]")) {
        return "section";
      }
      if (trimmed.startsWith('"') && !trimmed.endsWith("=")) {
        return "string";
      }
      if (
        trimmed === "=" ||
        trimmed === "[" ||
        trimmed === "]" ||
        trimmed === "," ||
        trimmed === "[[" ||
        trimmed === "]]"
      ) {
        return "punctuation";
      }
      if (/^-?\d/.test(trimmed)) {
        return "number";
      }
      if (/^(true|false)$/.test(trimmed)) {
        return "literal";
      }
      return "key";
    },
  );

const highlightTomlLine = (line: string) => {
  const [body, comment] = splitComment(line);
  return `${highlightTomlBody(body)}${comment ? token("comment", comment) : ""}`;
};

const highlightYamlBody = (line: string) =>
  highlightPattern(
    line,
    /^\s*(?:[\w$.-]+)(?=\s*:)|"([^"\\]|\\.)*"|'([^'\\]|\\.)*'|-?\b\d+(?:\.\d+)?\b|\b(?:true|false|null)\b|[:\[\]{},-]/g,
    (match) => {
      const trimmed = match.trim();
      if (/^[\w$.-]+$/.test(trimmed)) {
        return "key";
      }
      if (trimmed.startsWith('"') || trimmed.startsWith("'")) {
        return "string";
      }
      if (/^-?\d/.test(trimmed)) {
        return "number";
      }
      if (/^(true|false|null)$/.test(trimmed)) {
        return "literal";
      }
      return "punctuation";
    },
  );

const highlightYamlLine = (line: string) => {
  const [body, comment] = splitComment(line);
  return `${highlightYamlBody(body)}${comment ? token("comment", comment) : ""}`;
};

export const highlightCode = (code: string, language: Language) => {
  const highlighters = {
    json: highlightJsonLine,
    toml: highlightTomlLine,
    yaml: highlightYamlLine,
  };

  return code.split("\n").map(highlighters[language]).join("\n");
};

export type { Language };
