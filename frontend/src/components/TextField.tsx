import type { InputHTMLAttributes } from "react";

type TextFieldProps = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  name: string;
};

export function TextField({ label, name, className = "", ...props }: TextFieldProps) {
  return (
    <label className="block text-sm text-neutral-800" htmlFor={name}>
      <span className="mb-2 block">{label}</span>
      <input
        id={name}
        name={name}
        className={`h-10 w-full border border-neutral-300 bg-white px-3 text-sm text-black outline-none transition placeholder:text-neutral-400 focus:border-black ${className}`}
        {...props}
      />
    </label>
  );
}


