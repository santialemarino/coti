/* eslint-disable @next/next/no-html-link-for-pages -- plain anchors, as a <Link> renders them, are what the shell captures. */
import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { NAVIGATION_PENDING_DELAY_MS } from '@repo/ui/lib';
import {
  NavigationPendingProvider,
  NavigationProgressBar,
} from '@/app/(protected)/_components/navigation-pending';
import { ScreenFade } from '@/app/(protected)/_components/screen-fade';
import { useNavigationPending } from '@/hooks/use-navigation-pending';
import { usePendingRouter } from '@/hooks/use-pending-router';

/*
 * The shell's navigation feedback as the layout wires it. The router is a stand-in whose committed URL
 * the test moves by hand, the way Next commits a navigation.
 */

const router = vi.hoisted(() => ({
  url: { pathname: '/clients', search: '' },
  push: vi.fn(),
  replace: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  usePathname: () => router.url.pathname,
  useSearchParams: () => new URLSearchParams(router.url.search),
  useRouter: () => ({ push: router.push, replace: router.replace }),
}));
// Next's own React carries <ViewTransition>; the package the tests resolve does not yet.
vi.mock('react', async (importOriginal) => ({
  ...(await importOriginal<typeof import('react')>()),
  ViewTransition: ({ children }: { children: React.ReactNode }) => children,
}));

const shownDestinations: (string | null)[] = [];

function Probe() {
  const { pendingPathname, showProgress, busy } = useNavigationPending();
  const { push } = usePendingRouter();
  shownDestinations.push(pendingPathname);
  return (
    <>
      <output data-testid="probe">{JSON.stringify({ pendingPathname, showProgress, busy })}</output>
      <button type="button" onClick={() => push('/rfqs/r1')}>
        push
      </button>
    </>
  );
}

function Shell() {
  return (
    <NavigationPendingProvider>
      <NavigationProgressBar />
      <nav>
        <a href="/reports">reports</a>
        <a href="/rfqs">rfqs</a>
        <a href="/clients">clients</a>
        <a href="/clients?page=2">page 2</a>
        <a href="/export.csv" download>
          download
        </a>
        <a href="#main">skip</a>
        <a href="/api/invoices/i1/pdf?branch=b1">pdf</a>
      </nav>
      <ScreenFade>
        <Probe />
      </ScreenFade>
    </NavigationPendingProvider>
  );
}

function state() {
  return JSON.parse(screen.getByTestId('probe').textContent ?? '{}') as {
    pendingPathname: string | null;
    showProgress: boolean;
    busy: boolean;
  };
}

function bar() {
  return document.querySelector('[data-slot="navigation-progress"]');
}

function screenFrame() {
  return screen.getByTestId('probe').parentElement as HTMLElement;
}

// A link click as the browser dispatches it, its page load cancelled as Next's `<Link>` does.
function click(name: string, init: MouseEventInit = {}) {
  const link = screen.getByText(name);
  link.addEventListener('click', (event) => event.preventDefault(), { once: true });
  fireEvent.click(link, init);
}

function commit(rerender: (ui: React.ReactElement) => void, pathname: string, search = '') {
  router.url = { pathname, search };
  rerender(<Shell />);
}

const IDLE = { pendingPathname: null, showProgress: false, busy: false };

beforeEach(() => {
  vi.useFakeTimers();
  router.url = { pathname: '/clients', search: '' };
  window.history.replaceState(null, '', '/clients');
  router.push.mockClear();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('NavigationPendingProvider', () => {
  it('marks the destination on the click and shows progress only past the delay', () => {
    const { rerender } = render(<Shell />);
    click('reports');
    expect(state()).toEqual({ pendingPathname: '/reports', showProgress: false, busy: false });
    expect(bar()).toBeNull();

    act(() => vi.advanceTimersByTime(NAVIGATION_PENDING_DELAY_MS - 1));
    expect(state().showProgress).toBe(false);
    act(() => vi.advanceTimersByTime(1));
    expect(state()).toEqual({ pendingPathname: '/reports', showProgress: true, busy: true });
    expect(bar()).not.toBeNull();
    expect(screenFrame().getAttribute('aria-busy')).toBe('true');
    expect(screenFrame().className).toContain('opacity-60');
    expect(screenFrame().className).toContain('pointer-events-none');

    // The URL commits: everything goes in that same render, with no fade back.
    commit(rerender, '/reports');
    expect(state()).toEqual(IDLE);
    expect(bar()).toBeNull();
    expect(screenFrame().getAttribute('aria-busy')).toBeNull();
    expect(screenFrame().className).not.toContain('opacity-60');
    expect(screenFrame().className).not.toContain('transition-opacity');
  });

  it('shows nothing for a navigation that lands within the delay', () => {
    const { rerender } = render(<Shell />);
    click('reports');
    act(() => vi.advanceTimersByTime(NAVIGATION_PENDING_DELAY_MS / 2));
    commit(rerender, '/reports');
    act(() => vi.advanceTimersByTime(NAVIGATION_PENDING_DELAY_MS * 2));
    expect(state()).toEqual(IDLE);
    expect(bar()).toBeNull();
  });

  it('ends on a redirect as well: any move off the URL it left', () => {
    const { rerender } = render(<Shell />);
    click('reports');
    commit(rerender, '/session-ended');
    expect(state().pendingPathname).toBeNull();
  });

  it('forgets a finished navigation, so returning to the URL it left does not revive it', () => {
    const { rerender } = render(<Shell />);
    click('reports');
    commit(rerender, '/reports');
    commit(rerender, '/clients');
    act(() => vi.advanceTimersByTime(NAVIGATION_PENDING_DELAY_MS * 2));
    expect(state()).toEqual(IDLE);
  });

  it('lets a newer click take over, and a click back to the current page end it', () => {
    render(<Shell />);
    click('reports');
    click('rfqs');
    expect(state().pendingPathname).toBe('/rfqs');
    click('clients');
    act(() => vi.advanceTimersByTime(NAVIGATION_PENDING_DELAY_MS * 2));
    expect(state()).toEqual(IDLE);
  });

  it('ignores clicks that do not navigate this tab', () => {
    render(<Shell />);
    click('reports', { metaKey: true });
    click('reports', { ctrlKey: true });
    click('reports', { shiftKey: true });
    click('download');
    click('skip');
    click('pdf');
    act(() => vi.advanceTimersByTime(NAVIGATION_PENDING_DELAY_MS * 2));
    expect(state()).toEqual(IDLE);
  });

  // A filter keeps its screen usable: the bar says the result is coming, the screen stays as it is.
  it('gives a query-only navigation the bar but leaves the screen and the nav alone', () => {
    const { rerender } = render(<Shell />);
    click('page 2');
    act(() => vi.advanceTimersByTime(NAVIGATION_PENDING_DELAY_MS));
    expect(state()).toEqual({ pendingPathname: null, showProgress: true, busy: false });
    expect(screenFrame().getAttribute('aria-busy')).toBeNull();
    commit(rerender, '/clients', 'page=2');
    expect(state().showProgress).toBe(false);
  });

  it('follows back and forward', () => {
    const { rerender } = render(<Shell />);
    act(() => {
      window.history.pushState(null, '', '/rfqs');
      window.dispatchEvent(new PopStateEvent('popstate'));
    });
    expect(state().pendingPathname).toBe('/rfqs');
    commit(rerender, '/rfqs');
    expect(state().pendingPathname).toBeNull();
  });

  it('reports a programmatic push, and settles one whose transition ends without moving', () => {
    render(<Shell />);
    shownDestinations.length = 0;
    act(() => screen.getByText('push').click());
    expect(router.push).toHaveBeenCalledWith('/rfqs/r1', undefined);
    expect(shownDestinations).toContain('/rfqs/r1');
    // The stand-in router never commits, so the transition ends in place: settled, not stuck.
    act(() => vi.advanceTimersByTime(NAVIGATION_PENDING_DELAY_MS * 2));
    expect(state()).toEqual(IDLE);
  });
});
