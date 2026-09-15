import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, it, expect } from 'vitest';
// Bare specifier on purpose: nothing is emitted next to the sources any more
// (tsconfig.json sets `noEmit`), so this resolves to src/client.ts — the source of
// truth tsdown packages into the host-loaded lib/client.js.
import { LiveSpecHostCard } from '../src/client';

/**
 * Compact-safe host header regression test.
 *
 * jsdom is intentionally NOT a dependency of this package (adding one would
 * change package.json), so we render the host card with react-dom/server and
 * parse the produced markup. The `style` attributes asserted below are the real
 * inline styles React emitted for the header / title / action group — not
 * generated expectations.
 */

interface ParsedElement {
  tag: string;
  attrs: Record<string, string>;
  style: Record<string, string>;
  children: ParsedElement[];
  text: string;
}

const VOID_TAGS = new Set(['input', 'img', 'br', 'hr', 'meta', 'link']);

function parseAttrs(raw: string): Record<string, string> {
  const attrs: Record<string, string> = {};
  const re = /([a-zA-Z_:][-a-zA-Z0-9_:.]*)(?:="([^"]*)")?/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(raw))) {
    attrs[m[1]] = m[2] === undefined ? '' : m[2];
  }
  return attrs;
}

function parseStyle(style: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const decl of style.split(';')) {
    const idx = decl.indexOf(':');
    if (idx === -1) continue;
    const key = decl.slice(0, idx).trim().toLowerCase();
    const value = decl.slice(idx + 1).trim();
    if (key) out[key] = value;
  }
  return out;
}

function parseMarkup(html: string): ParsedElement {
  const root: ParsedElement = { tag: '#root', attrs: {}, style: {}, children: [], text: '' };
  const stack: ParsedElement[] = [root];
  const tagRe = /<\/?([a-zA-Z][a-zA-Z0-9-]*)((?:\s+[^<>]*?)?)\/?>/g;
  let last = 0;
  let m: RegExpExecArray | null;

  while ((m = tagRe.exec(html))) {
    const raw = m[0];
    const tag = m[1];
    const pendingText = html.slice(last, m.index);
    if (pendingText) stack[stack.length - 1].text += pendingText;
    last = m.index + raw.length;

    if (raw.startsWith('</')) {
      stack.pop();
      continue;
    }

    const element: ParsedElement = {
      tag,
      attrs: parseAttrs(m[2] || ''),
      style: {},
      children: [],
      text: '',
    };
    element.style = parseStyle(element.attrs.style || '');
    stack[stack.length - 1].children.push(element);

    if (!raw.endsWith('/>') && !VOID_TAGS.has(tag)) stack.push(element);
  }

  return root;
}

/** 0 is serialized by React as "0"; accept "0px" too in case of normalization. */
const isZero = (value: string | undefined) => value === '0' || value === '0px';

function renderHostHeader(props: Record<string, unknown>) {
  const html = renderToStaticMarkup(React.createElement(LiveSpecHostCard, props));
  const root = parseMarkup(html);
  const container = root.children[0];
  const header = container.children[0];
  const leftGroup = header.children[0];
  const actions = header.children[1];
  const title = leftGroup.children[0];
  const cwdBadge = leftGroup.children.find(
    (c) => c.attrs['data-testid'] === 'header-cwd-badge'
  );
  const button = actions.children[0];
  return { html, container, header, leftGroup, actions, title, cwdBadge, button };
}

describe('LiveSpecHostCard compact-safe header', () => {
  const cwd = 'D:\\myprogram\\experience\\siruoning\\Ai_medbox';

  it('keeps every required testid and label in the rendered header', () => {
    const { html } = renderHostHeader({
      initialMeta: { sessionId: 'session-compact', cwd },
    });

    expect(html).toContain('data-testid="live-spec-host-container"');
    expect(html).toContain('data-testid="header-cwd-badge"');
    expect(html).toContain('data-testid="fullscreen-toggle-btn"');
    expect(html).toContain(`工作区: ${cwd}`);
    expect(html).toContain('⤢ 展开');
  });

  it('renders a header node that can shrink instead of squeezing its children', () => {
    const { header, leftGroup } = renderHostHeader({
      initialMeta: { sessionId: 'session-compact', cwd },
    });

    expect(header).toBeDefined();
    expect(header.tag).toBe('div');
    // real inline styles straight from the render output
    expect(header.style['display']).toBe('flex');
    expect(isZero(header.style['min-width'])).toBe(true);
    expect(header.style['gap']).toBe('8px');

    expect(leftGroup).toBeDefined();
    expect(isZero(leftGroup.style['min-width'])).toBe(true);
    expect(leftGroup.style['overflow']).toBe('hidden');
  });

  it('gives the title span nowrap + ellipsis + min-width:0 so long cwd text cannot force a wrap', () => {
    const { title } = renderHostHeader({
      initialMeta: { sessionId: 'session-compact', cwd },
    });

    expect(title).toBeDefined();
    expect(title.tag).toBe('span');
    expect(title.text).toBe(`工作区: ${cwd}`);
    expect(title.style['white-space']).toBe('nowrap');
    expect(title.style['text-overflow']).toBe('ellipsis');
    expect(isZero(title.style['min-width'])).toBe(true);
    expect(title.style['overflow']).toBe('hidden');
  });

  it('pins the right-hand action group with flex-shrink:0 so ⤢ 展开 stays fully visible', () => {
    const { actions, button } = renderHostHeader({
      initialMeta: { sessionId: 'session-compact', cwd },
    });

    expect(actions).toBeDefined();
    expect(actions.tag).toBe('div');
    expect(actions.style['flex-shrink']).toBe('0');
    expect(actions.style['display']).toBe('flex');

    expect(button).toBeDefined();
    expect(button.tag).toBe('button');
    expect(button.attrs['data-testid']).toBe('fullscreen-toggle-btn');
    expect(button.text).toBe('⤢ 展开');
    expect(button.style['flex-shrink']).toBe('0');
    expect(button.style['white-space']).toBe('nowrap');
  });

  it('applies the same compact-safe styles to the cwd badge and to the dag_id variant', () => {
    const withCwdBadge = renderHostHeader({
      initialMeta: { sessionId: 'session-compact', cwd },
    });
    expect(withCwdBadge.cwdBadge).toBeDefined();
    expect(withCwdBadge.cwdBadge!.style['text-overflow']).toBe('ellipsis');
    expect(withCwdBadge.cwdBadge!.style['white-space']).toBe('nowrap');
    expect(isZero(withCwdBadge.cwdBadge!.style['min-width'])).toBe(true);

    const withDagId = renderHostHeader({
      initialSpec: {
        version: '1.0.0',
        title: 'Active Project DAG',
        dag_id: 'dag-active-run-with-a-very-long-identifier',
        tasks: [{ id: 't1', title: 'Task 1', state: 'running' }],
      },
      initialMeta: { sessionId: 'session-compact', cwd },
    });
    expect(withDagId.html).toContain('dag-active-run-with-a-very-long-identifier');
    expect(withDagId.title.style['white-space']).toBe('nowrap');
    expect(withDagId.title.style['text-overflow']).toBe('ellipsis');
    expect(isZero(withDagId.title.style['min-width'])).toBe(true);
    // dag_id badge (second child of the left group) must be shrinkable too
    const dagBadge = withDagId.leftGroup.children[1];
    expect(dagBadge).toBeDefined();
    expect(dagBadge.style['text-overflow']).toBe('ellipsis');
    expect(dagBadge.style['white-space']).toBe('nowrap');
    expect(isZero(dagBadge.style['min-width'])).toBe(true);
    expect(dagBadge.style['max-width']).toBe('240px');
  });
});
