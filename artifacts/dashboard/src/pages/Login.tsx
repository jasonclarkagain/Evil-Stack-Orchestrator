import { useState, type FormEvent } from "react";
import { useAuth } from "@/context/auth";

export default function LoginPage() {
  const { login } = useAuth();
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await login(password);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen bg-[#0d1117] flex items-center justify-center">
      <div className="w-full max-w-sm space-y-6 px-6">
        <div className="text-center space-y-1">
          <p className="text-xs font-mono tracking-widest text-cyan-400 uppercase">
            Orchestrator
          </p>
          <h1 className="text-xl font-mono font-semibold text-white">
            Operator Access
          </h1>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1">
            <label className="block text-xs font-mono text-gray-400 uppercase tracking-wider">
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              autoFocus
              className="w-full bg-[#161b22] border border-[#30363d] rounded px-3 py-2 text-sm font-mono text-white placeholder-gray-600 focus:outline-none focus:border-cyan-500 transition-colors"
              placeholder="Enter operator password"
            />
          </div>

          {error && (
            <p className="text-xs font-mono text-red-400">{error}</p>
          )}

          <button
            type="submit"
            disabled={loading}
            className="w-full bg-cyan-600 hover:bg-cyan-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-mono py-2 rounded transition-colors"
          >
            {loading ? "Authenticating..." : "Sign In"}
          </button>
        </form>
      </div>
    </div>
  );
}
