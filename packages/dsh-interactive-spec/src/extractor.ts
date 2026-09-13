import type { LiveSpecDoc, SpecTask } from '@agentflow/live-spec-core';
import type { ExtractOptions } from './types';

const CODE_BLOCK_REGEX = /`{3,}[ \t]*(?:json:)?agentflow-spec[^\r\n]*\r?\n([\s\S]*?)`{3,}/gi;

/**
 * Remove JS comments (// and /* *\/) and trailing commas outside of string literals.
 */
function cleanJsonCommentsAndCommas(input: string): string {
  let insideString = false;
  let stringChar = '';
  let isEscaped = false;
  let result = '';
  let i = 0;
  const len = input.length;

  while (i < len) {
    const char = input[i];
    const nextChar = input[i + 1];

    if (insideString) {
      result += char;
      if (isEscaped) {
        isEscaped = false;
      } else if (char === '\\') {
        isEscaped = true;
      } else if (char === stringChar) {
        insideString = false;
      }
      i++;
      continue;
    }

    if (char === '"' || char === "'") {
      insideString = true;
      stringChar = char;
      result += char;
      i++;
      continue;
    }

    // Single-line comment: // ...
    if (char === '/' && nextChar === '/') {
      i += 2;
      while (i < len && input[i] !== '\n' && input[i] !== '\r') {
        i++;
      }
      continue;
    }

    // Multi-line comment: /* ... */
    if (char === '/' && nextChar === '*') {
      i += 2;
      while (i < len && !(input[i] === '*' && input[i + 1] === '/')) {
        i++;
      }
      i += 2; // skip */
      continue;
    }

    result += char;
    i++;
  }

  // Remove trailing commas before } or ]
  return result.replace(/,\s*([}\]])/g, '$1');
}

/**
 * Tolerant JSON parse that cleans comments and trailing commas if raw parse fails.
 */
export function tolerantJsonParse(raw: string): unknown | null {
  const trimmed = raw.trim();
  if (!trimmed) return null;

  try {
    return JSON.parse(trimmed);
  } catch {
    // Attempt cleanup
    try {
      const sanitized = cleanJsonCommentsAndCommas(trimmed);
      return JSON.parse(sanitized);
    } catch {
      return null;
    }
  }
}

/**
 * Validates and normalizes raw parsed object into a clean LiveSpecDoc.
 */
export function normalizeLiveSpec(rawObj: unknown): LiveSpecDoc | null {
  if (!rawObj || typeof rawObj !== 'object' || Array.isArray(rawObj)) {
    return null;
  }

  const record = rawObj as Record<string, unknown>;

  if (!Array.isArray(record.tasks)) {
    return null;
  }

  const tasks: SpecTask[] = [];
  for (const item of record.tasks) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) {
      continue;
    }
    const t = item as Record<string, unknown>;
    if (t.id === undefined || t.id === null) {
      continue;
    }

    const task: SpecTask = {
      ...t,
      id: String(t.id),
      title: typeof t.title === 'string' ? t.title : String(t.id),
      depends_on: Array.isArray(t.depends_on) ? t.depends_on.map(String) : undefined,
      acceptance_criteria: Array.isArray(t.acceptance_criteria)
        ? t.acceptance_criteria.map(String)
        : undefined,
      tags: Array.isArray(t.tags) ? t.tags.map(String) : undefined,
      output_files: Array.isArray(t.output_files) ? t.output_files.map(String) : undefined,
      estimated_hours: typeof t.estimated_hours === 'number' ? t.estimated_hours : undefined,
      priority: typeof t.priority === 'number' ? t.priority : undefined,
      assigned_worker: typeof t.assigned_worker === 'string' ? t.assigned_worker : undefined,
      state: typeof t.state === 'string' ? (t.state as SpecTask['state']) : undefined,
    };

    tasks.push(task);
  }

  const doc: LiveSpecDoc = {
    version: typeof record.version === 'string' ? record.version : '1.0.0',
    title: typeof record.title === 'string' ? record.title : 'Untitled Spec',
    tasks,
  };

  if (typeof record.dag_id === 'string') doc.dag_id = record.dag_id;
  if (typeof record.namespace_id === 'string') doc.namespace_id = record.namespace_id;
  if (typeof record.concurrency === 'number') doc.concurrency = record.concurrency;
  if (record.settings && typeof record.settings === 'object' && !Array.isArray(record.settings)) {
    doc.settings = record.settings as LiveSpecDoc['settings'];
  }
  if (record.metadata && typeof record.metadata === 'object' && !Array.isArray(record.metadata)) {
    doc.metadata = record.metadata as Record<string, unknown>;
  }

  return doc;
}

/**
 * Extracts all valid LiveSpecDoc blocks from Markdown content.
 */
export function extractAllSpecsFromMarkdown(markdown: string): LiveSpecDoc[] {
  if (!markdown || typeof markdown !== 'string') {
    return [];
  }

  const specs: LiveSpecDoc[] = [];
  const regex = new RegExp(CODE_BLOCK_REGEX.source, CODE_BLOCK_REGEX.flags);

  let match: RegExpExecArray | null;
  while ((match = regex.exec(markdown)) !== null) {
    const rawContent = match[1];
    const parsed = tolerantJsonParse(rawContent);
    if (parsed) {
      const doc = normalizeLiveSpec(parsed);
      if (doc) {
        specs.push(doc);
      }
    }
  }

  return specs;
}

/**
 * Extracts a LiveSpecDoc from Markdown content.
 * Defaults to the last (latest revision) valid spec block if multiple exist.
 */
export function extractSpecFromMarkdown(
  markdown: string,
  options?: ExtractOptions
): LiveSpecDoc | null {
  const specs = extractAllSpecsFromMarkdown(markdown);
  if (specs.length === 0) {
    return null;
  }

  if (options?.pick === 'first') {
    return specs[0];
  }

  return specs[specs.length - 1];
}
