import type { InputHTMLAttributes } from "react";

type TextFieldProps = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  name: string;
};

export function TextField({ label, name, className = "", ...props }: TextFieldProps) {
  return (
    <label className="block text-sm text-neutral-700 dark:text-neutral-300" htmlFor={name}>
      <span className="mb-2 block font-medium tracking-wide text-neutral-600 dark:text-neutral-400">{label}</span>
      <input
        id={name}
        name={name}
        className={`h-10 w-full rounded-lg border border-neutral-300 dark:border-white/10 bg-white dark:bg-zinc-900/50 px-3 text-sm text-neutral-900 dark:text-white outline-none transition-all placeholder:text-neutral-400 dark:placeholder:text-neutral-500 focus:border-red-500/60 focus:ring-2 focus:ring-red-500/20 ${className}`}
        {...props}
      />
    </label>
  );
}




