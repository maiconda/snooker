import type { ReactNode } from "react";

type AuthShellProps = {
  title: string;
  children: ReactNode;
  footer: ReactNode;
};

export function AuthShell({ title, children, footer }: AuthShellProps) {
  return (
    <main className="relative flex min-h-screen items-center justify-center bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-red-100/40 via-zinc-50 to-zinc-50 dark:from-red-950/30 dark:via-zinc-950 dark:to-zinc-950 px-6 py-12 text-neutral-900 dark:text-white">
      <div className="absolute inset-0 bg-[linear-gradient(to_bottom,rgba(0,0,0,0.02)_1px,transparent_1px)] dark:bg-[linear-gradient(to_bottom,rgba(255,255,255,0.01)_1px,transparent_1px)] bg-[size:100%_40px] pointer-events-none" />
      <section className="w-full max-w-[400px] rounded-2xl border border-neutral-200/80 dark:border-white/10 bg-white/80 dark:bg-zinc-900/60 p-8 backdrop-blur-xl shadow-xl dark:shadow-2xl shadow-neutral-200/40 dark:shadow-black/80">
        <div className="mb-6 text-center">
          <img src="/logo.png" alt="Snooker Club" className="mx-auto h-24 w-auto object-contain mb-4 mt-2" />
          <h1 className="mt-2 text-2xl font-bold tracking-tight text-neutral-900 dark:text-white">{title}</h1>
        </div>
        
        {children}
        
        <div className="mt-8 border-t border-neutral-100 dark:border-white/5 pt-4 text-center text-xs text-neutral-500 dark:text-neutral-400">
          {footer}
        </div>
      </section>
    </main>
  );
}



