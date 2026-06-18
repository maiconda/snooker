import { useState } from "react";
import { useAuth } from "../auth/AuthProvider";
import { Button } from "../components/Button";

export function HomePage() {
  const { logout } = useAuth();
  const [loading, setLoading] = useState(false);

  const handleLogout = async () => {
    setLoading(true);
    try {
      await logout();
    } catch (error) {
      console.error("Erro ao fazer logout:", error);
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="flex min-h-screen flex-col items-center justify-center bg-white p-6" aria-label="Pagina inicial">
      <div className="w-full max-w-[360px]">
        <Button onClick={handleLogout} disabled={loading}>
          {loading ? "Saindo..." : "Logout"}
        </Button>
      </div>
    </main>
  );
}
