import { FormEvent, useMemo, useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { AuthShell } from "../components/AuthShell";
import { Button } from "../components/Button";
import { Notice } from "../components/Notice";
import { TextField } from "../components/TextField";
import { Link, navigate } from "../lib/router";

export function SignupPage() {
  const auth = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  const passwordIssue = useMemo(() => validatePassword(password), [password]);
  const canSubmit = email.length > 0 && password.length > 0 && !passwordIssue && !submitting;

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLocalError(null);

    if (passwordIssue) {
      setLocalError(passwordIssue);
      return;
    }

    setSubmitting(true);
    try {
      await auth.signup({ email, password });
      navigate("/");
    } catch (error) {
      setLocalError(error instanceof Error ? error.message : "Falha ao cadastrar.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthShell
      title="Criar conta"
      footer={
        <span>
          Já tem conta?{" "}
          <Link className="text-red-600 dark:text-red-400 font-semibold hover:text-red-500 dark:hover:text-red-300 transition-colors hover:underline underline-offset-4" to="/login">
            Entrar
          </Link>
        </span>
      }
    >
      <form className="space-y-4" onSubmit={handleSubmit}>
        <TextField label="Email" name="email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required />
        <TextField
          label="Senha"
          name="password"
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          required
        />

        <Notice message={localError ?? auth.error} />

        <Button type="submit" disabled={!canSubmit}>
          {submitting ? "Criando..." : "Criar conta"}
        </Button>
      </form>
    </AuthShell>
  );
}

function validatePassword(password: string): string | null {
  if (!password) {
    return null;
  }
  if (password.length < 8 || password.length > 72) {
    return "A senha deve ter entre 8 e 72 caracteres.";
  }
  if (!/[A-Z]/.test(password)) {
    return "A senha deve conter pelo menos uma letra maiuscula.";
  }
  if (!/[a-z]/.test(password)) {
    return "A senha deve conter pelo menos uma letra minuscula.";
  }
  if (!/[0-9]/.test(password)) {
    return "A senha deve conter pelo menos um numero.";
  }
  if (!/[!@#$%^&*()_+\-=[\]{};':",./<>?~`|\\ ]/.test(password)) {
    return "A senha deve conter pelo menos um caractere especial.";
  }
  return null;
}
