import { useEffect, useSyncExternalStore, type AnchorHTMLAttributes, type ReactNode } from "react";

const listeners = new Set<() => void>();

function notify() {
  listeners.forEach((listener) => listener());
}

export function navigate(path: string) {
  if (window.location.pathname === path) {
    return;
  }
  window.history.pushState(null, "", path);
  notify();
}

export function usePathname() {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      window.addEventListener("popstate", listener);
      return () => {
        listeners.delete(listener);
        window.removeEventListener("popstate", listener);
      };
    },
    () => window.location.pathname,
    () => "/"
  );
}

export function RouterEvents() {
  useEffect(() => {
    const originalPushState = window.history.pushState;
    const originalReplaceState = window.history.replaceState;

    window.history.pushState = function pushState(...args) {
      originalPushState.apply(this, args);
      notify();
    };

    window.history.replaceState = function replaceState(...args) {
      originalReplaceState.apply(this, args);
      notify();
    };

    return () => {
      window.history.pushState = originalPushState;
      window.history.replaceState = originalReplaceState;
    };
  }, []);

  return null;
}

type LinkProps = AnchorHTMLAttributes<HTMLAnchorElement> & {
  to: string;
  children: ReactNode;
};

export function Link({ to, children, onClick, ...props }: LinkProps) {
  return (
    <a
      href={to}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented && event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey) {
          event.preventDefault();
          navigate(to);
        }
      }}
      {...props}
    >
      {children}
    </a>
  );
}
