import type { ButtonHTMLAttributes, ReactNode } from "react";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  children: ReactNode;
  variant?: "solid" | "outline" | "ghost";
};

export function Button({ children, variant = "solid", className = "", ...props }: ButtonProps) {
  const base =
    "inline-flex h-10 w-full items-center justify-center rounded-lg border px-4 text-sm font-semibold tracking-wide transition-all duration-200 ease-out focus:outline-none focus:ring-2 focus:ring-red-500/40 disabled:cursor-not-allowed disabled:opacity-40 active:scale-[0.98]";
  
  const variants = {
    solid: "border-red-600 bg-red-600 text-white hover:bg-red-500 hover:border-red-500 shadow-md shadow-red-950/10 dark:border-red-700 dark:bg-red-700 dark:hover:bg-red-600 dark:hover:border-red-600",
    outline: "border-neutral-300 dark:border-white/10 bg-white/40 dark:bg-white/5 text-neutral-800 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-white/10 hover:border-neutral-400 dark:hover:border-white/20 hover:text-neutral-900 dark:hover:text-white",
    ghost: "border-transparent bg-transparent text-neutral-500 dark:text-neutral-400 hover:text-neutral-800 dark:hover:text-white hover:bg-neutral-100 dark:hover:bg-white/5"
  };

  return (
    <button className={`${base} ${variants[variant]} ${className}`} {...props}>
      {children}
    </button>
  );
}



