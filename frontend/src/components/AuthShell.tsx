import type { ReactNode } from "react";

type AuthShellProps = {
  title: string;
  children: ReactNode;
  footer: ReactNode;
};

export function AuthShell({ title, children, footer }: AuthShellProps) {
  return (
    <main className="flex min-h-screen items-center justify-center bg-white px-6 py-10 text-black">
      <section className="w-full max-w-[360px]">
        <h1 className="mb-8 text-xl font-medium tracking-normal text-black">{title}</h1>
        {children}
        <div className="mt-6 text-sm text-neutral-600">{footer}</div>
      </section>
    </main>
  );
}


