import { FormEvent, useEffect, useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { consumeGoogleCallback, startGoogleLogin } from "../auth/google";
import { AuthShell } from "../components/AuthShell";
import { Button } from "../components/Button";
import { Notice } from "../components/Notice";
import { TextField } from "../components/TextField";
import { Link, navigate } from "../lib/router";

export function LoginPage() {
  const auth = useAuth();
  const { loginWithGoogleToken } = auth;
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);

  useEffect(() => {
    const callback = consumeGoogleCallback();
    if (!callback.ok) {
      if (callback.error) {
        setLocalError(callback.error);
      }
      return;
    }

    setSubmitting(true);
    loginWithGoogleToken(callback.idToken)
      .then(() => navigate("/"))
      .catch((error: unknown) => {
        setLocalError(error instanceof Error ? error.message : "Falha ao entrar com Google.");
      })
      .finally(() => setSubmitting(false));
  }, [loginWithGoogleToken]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setLocalError(null);
    setSubmitting(true);

    try {
      await auth.login({ email, password });
      navigate("/");
    } catch (error) {
      setLocalError(error instanceof Error ? error.message : "Falha ao entrar.");
    } finally {
      setSubmitting(false);
    }
  }

  function handleGoogleLogin() {
    setLocalError(null);
    try {
      startGoogleLogin();
    } catch (error) {
      setLocalError(error instanceof Error ? error.message : "Google indisponivel.");
    }
  }

  return (
    <AuthShell
      title="Entrar"
      footer={
        <span>
          Ainda não tem conta?{" "}
          <Link className="text-red-600 dark:text-red-400 font-semibold hover:text-red-500 dark:hover:text-red-300 transition-colors hover:underline underline-offset-4" to="/cadastro">
            Criar conta
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
          autoComplete="current-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          required
        />

        <Notice message={localError ?? auth.error} />

        <Button type="submit" disabled={submitting}>
          {submitting ? "Entrando..." : "Entrar"}
        </Button>
        <Button type="button" variant="outline" onClick={handleGoogleLogin} disabled={submitting}>
          Continuar com Google
        </Button>
      </form>
    </AuthShell>
  );
}
