import { describe, it, expect } from 'vitest';
import { extractSpecFromMarkdown, extractAllSpecsFromMarkdown, } from '../src/extractor';
describe('extractSpecFromMarkdown', () => {
    describe('Single code block', () => {
        it('extracts spec from ```agentflow-spec code block', () => {
            const md = `
# Project Plan

Here is the plan for our backend tasks:

\`\`\`agentflow-spec
{
  "version": "1.0.0",
  "title": "Backend Migration",
  "dag_id": "dag-backend-1",
  "tasks": [
    {
      "id": "task-1",
      "title": "Database Schema Setup",
      "estimated_hours": 3,
      "assigned_worker": "backend-dev"
    },
    {
      "id": "task-2",
      "title": "User Authentication API",
      "depends_on": ["task-1"],
      "estimated_hours": 5
    }
  ]
}
\`\`\`

End of plan.
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.title).toBe('Backend Migration');
            expect(spec?.version).toBe('1.0.0');
            expect(spec?.dag_id).toBe('dag-backend-1');
            expect(spec?.tasks).toHaveLength(2);
            expect(spec?.tasks[0].id).toBe('task-1');
            expect(spec?.tasks[1].depends_on).toEqual(['task-1']);
        });
        it('extracts spec from ```json:agentflow-spec code block', () => {
            const md = `
\`\`\`json:agentflow-spec
{
  "version": "1.1.0",
  "title": "Frontend Redesign",
  "tasks": [
    {
      "id": "t-ui-1",
      "title": "Implement Button Component"
    }
  ]
}
\`\`\`
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.title).toBe('Frontend Redesign');
            expect(spec?.version).toBe('1.1.0');
            expect(spec?.tasks).toHaveLength(1);
            expect(spec?.tasks[0].id).toBe('t-ui-1');
        });
        it('handles case-insensitive block tags', () => {
            const md = `
\`\`\`AGENTFLOW-SPEC
{
  "title": "Case Test",
  "tasks": [{ "id": "1", "title": "Test" }]
}
\`\`\`
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.title).toBe('Case Test');
        });
    });
    describe('Multiple code blocks', () => {
        const multiMd = `
Initial proposal:
\`\`\`agentflow-spec
{
  "version": "1.0.0",
  "title": "Initial Plan",
  "tasks": [
    { "id": "t-init", "title": "Setup initial structure" }
  ]
}
\`\`\`

After discussion, here is the updated plan:
\`\`\`json:agentflow-spec
{
  "version": "2.0.0",
  "title": "Revised Plan",
  "tasks": [
    { "id": "t-rev-1", "title": "Setup structure" },
    { "id": "t-rev-2", "title": "Add testing harness", "depends_on": ["t-rev-1"] }
  ]
}
\`\`\`
`;
        it('defaults to picking the latest (last) valid spec block', () => {
            const spec = extractSpecFromMarkdown(multiMd);
            expect(spec).not.toBeNull();
            expect(spec?.version).toBe('2.0.0');
            expect(spec?.title).toBe('Revised Plan');
            expect(spec?.tasks).toHaveLength(2);
        });
        it('allows picking the first spec block when specified', () => {
            const spec = extractSpecFromMarkdown(multiMd, { pick: 'first' });
            expect(spec).not.toBeNull();
            expect(spec?.version).toBe('1.0.0');
            expect(spec?.title).toBe('Initial Plan');
            expect(spec?.tasks).toHaveLength(1);
        });
        it('extracts all valid spec blocks in order with extractAllSpecsFromMarkdown', () => {
            const allSpecs = extractAllSpecsFromMarkdown(multiMd);
            expect(allSpecs).toHaveLength(2);
            expect(allSpecs[0].title).toBe('Initial Plan');
            expect(allSpecs[1].title).toBe('Revised Plan');
        });
    });
    describe('Invalid formats and edge cases', () => {
        it('returns null for markdown without any matching code blocks', () => {
            const md = '# Just some markdown\n```js\nconst a = 1;\n```';
            expect(extractSpecFromMarkdown(md)).toBeNull();
            expect(extractAllSpecsFromMarkdown(md)).toEqual([]);
        });
        it('returns null for empty or non-string input', () => {
            expect(extractSpecFromMarkdown('')).toBeNull();
            // @ts-expect-error test non-string runtime safety
            expect(extractSpecFromMarkdown(null)).toBeNull();
            // @ts-expect-error test non-string runtime safety
            expect(extractSpecFromMarkdown(undefined)).toBeNull();
        });
        it('returns null for broken unparseable JSON block', () => {
            const md = `
\`\`\`agentflow-spec
{ this is clearly not JSON at all: [{{{
\`\`\`
`;
            expect(extractSpecFromMarkdown(md)).toBeNull();
        });
        it('skips invalid blocks and extracts the valid one when multiple blocks exist', () => {
            const md = `
\`\`\`agentflow-spec
{ broken: json ...
\`\`\`

\`\`\`agentflow-spec
{
  "title": "Working Spec",
  "tasks": [{ "id": "t-ok", "title": "Good Task" }]
}
\`\`\`
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.title).toBe('Working Spec');
            expect(spec?.tasks[0].id).toBe('t-ok');
        });
        it('rejects JSON without tasks or with non-array tasks', () => {
            const noTasks = `
\`\`\`agentflow-spec
{
  "title": "No Tasks Here"
}
\`\`\`
`;
            expect(extractSpecFromMarkdown(noTasks)).toBeNull();
            const invalidTasks = `
\`\`\`agentflow-spec
{
  "title": "Invalid Tasks",
  "tasks": "not-an-array"
}
\`\`\`
`;
            expect(extractSpecFromMarkdown(invalidTasks)).toBeNull();
        });
    });
    describe('Fault-tolerant parsing', () => {
        it('tolerates trailing commas in JSON', () => {
            const md = `
\`\`\`agentflow-spec
{
  "title": "Trailing Commas",
  "tasks": [
    {
      "id": "t-1",
      "title": "Task 1",
    },
  ],
}
\`\`\`
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.title).toBe('Trailing Commas');
            expect(spec?.tasks).toHaveLength(1);
            expect(spec?.tasks[0].id).toBe('t-1');
        });
        it('tolerates single-line and multi-line comments in JSON', () => {
            const md = `
\`\`\`json:agentflow-spec
{
  // Project settings
  "title": "Commented Spec",
  /* Multi-line
     explanation */
  "tasks": [
    {
      "id": "t-comment",
      "title": "Clean Task" // inline comment
    }
  ]
}
\`\`\`
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.title).toBe('Commented Spec');
            expect(spec?.tasks).toHaveLength(1);
        });
        it('tolerates markdown with leading/trailing spaces and mixed line endings', () => {
            const md = "   \r\n```agentflow-spec\r\n{\r\n  \"title\": \"CRLF Spec\",\r\n  \"tasks\": [{\"id\": \"1\", \"title\": \"One\"}]\r\n}\r\n```\r\n   ";
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.title).toBe('CRLF Spec');
            expect(spec?.tasks).toHaveLength(1);
        });
        it('normalizes missing version or title with sensible defaults', () => {
            const md = `
\`\`\`agentflow-spec
{
  "tasks": [
    { "id": "t-bare", "title": "Bare Task" }
  ]
}
\`\`\`
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.version).toBe('1.0.0');
            expect(spec?.title).toBe('Untitled Spec');
            expect(spec?.tasks[0].id).toBe('t-bare');
        });
        it('does not corrupt URLs containing // inside strings', () => {
            const md = `
\`\`\`agentflow-spec
{
  "title": "URL Spec",
  "metadata": {
    "repo": "https://github.com/test/repo",
    "docs": "http://example.com/api"
  },
  "tasks": [
    { "id": "1", "title": "Check https://github.com" }
  ]
}
\`\`\`
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.metadata?.repo).toBe('https://github.com/test/repo');
            expect(spec?.metadata?.docs).toBe('http://example.com/api');
            expect(spec?.tasks[0].title).toBe('Check https://github.com');
        });
        it('filters out invalid task objects while preserving valid ones', () => {
            const md = `
\`\`\`agentflow-spec
{
  "title": "Mixed Tasks",
  "tasks": [
    null,
    "string-task-not-obj",
    { "id": "valid-1", "title": "First Valid" },
    { "no_id": true },
    { "id": "valid-2", "title": "Second Valid", "depends_on": ["valid-1"] }
  ]
}
\`\`\`
`;
            const spec = extractSpecFromMarkdown(md);
            expect(spec).not.toBeNull();
            expect(spec?.tasks).toHaveLength(2);
            expect(spec?.tasks[0].id).toBe('valid-1');
            expect(spec?.tasks[1].id).toBe('valid-2');
        });
    });
});
