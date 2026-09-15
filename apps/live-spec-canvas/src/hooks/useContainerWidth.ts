import { useEffect, useRef, useState } from 'react';
import type { RefObject } from 'react';

/**
 * Width (in CSS px) below which the live-spec canvas switches to its compact,
 * wrap-friendly layout. The canvas is embedded inside a DSH right pane iframe,
 * so `window.innerWidth` mirrors the host window and is NOT a reliable signal —
 * we always measure the canvas container element itself.
 */
export const COMPACT_BREAKPOINT = 900;

export interface ContainerWidthState<T extends HTMLElement> {
  /** Attach this ref to the outermost canvas element. */
  containerRef: RefObject<T>;
  /** Measured width of the container in CSS px (0 before the first measurement). */
  width: number;
  /** True only once a real measurement below COMPACT_BREAKPOINT has been taken. */
  isCompact: boolean;
}

/**
 * Observes the size of the element the returned ref is attached to.
 * Uses ResizeObserver when available, with a window-resize fallback.
 */
export function useContainerWidth<T extends HTMLElement = HTMLDivElement>(
  breakpoint: number = COMPACT_BREAKPOINT
): ContainerWidthState<T> {
  const containerRef = useRef<T>(null);
  const [width, setWidth] = useState<number>(0);

  useEffect(() => {
    const element = containerRef.current;
    if (!element) return;

    const measure = () => {
      const target = containerRef.current;
      if (!target) return;
      const next = target.getBoundingClientRect().width;
      setWidth((prev) => (Math.abs(prev - next) < 0.5 ? prev : next));
    };

    measure();

    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure);
      return () => window.removeEventListener('resize', measure);
    }

    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, []);

  return {
    containerRef,
    width,
    // width === 0 means "not measured yet" -> keep the wide layout to avoid a
    // flash of compact styling on first paint.
    isCompact: width > 0 && width < breakpoint,
  };
}
