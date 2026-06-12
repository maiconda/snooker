import type { ButtonHTMLAttributes, ReactNode } from "react";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  children: ReactNode;
  variant?: "solid" | "outline" | "ghost";
};

export function Button({ children, variant = "solid", className = "", ...props }: ButtonProps) {
  const base =
    "inline-flex h-10 w-full items-center justify-center border px-3 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-50";
  const variants = {
    solid: "border-black bg-black text-white hover:bg-neutral-800",
    outline: "border-neutral-300 bg-white text-neutral-900 hover:border-neutral-500",
    ghost: "border-transparent bg-white text-neutral-700 hover:text-black"
  };

  return (
    <button className={`${base} ${variants[variant]} ${className}`} {...props}>
      {children}
    </button>
  );
}


