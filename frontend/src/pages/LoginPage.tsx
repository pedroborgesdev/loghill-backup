import { Eye, EyeOff, Lock, LogIn } from "lucide-react";
import { useState, type FormEvent } from "react";
import { APIError } from "../types/api";
import { useAuth } from "../auth/AuthProvider";
import { Button, Input } from "../components/ui";

export function LoginPage() {
  const { login } = useAuth();
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (!password.trim() || submitting) return;
    setSubmitting(true);
    setError("");
    try {
      await login(password);
    } catch (requestError) {
      if (requestError instanceof APIError && requestError.status === 401) {
        setError("Invalid password. Please try again.");
      } else {
        setError(requestError instanceof Error ? requestError.message : "Unable to sign in.");
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="grid min-h-[100dvh] place-items-center bg-[#0c0c0f] px-4 py-8 text-zinc-100">
      <div className="w-full max-w-md">
        <div className="mb-8 flex flex-col items-center text-center">
          <img
            src="/loghill.png"
            alt="LogHill"
            className="h-36 w-36 object-contain sm:h-44 sm:w-44"
          />
          <h1 className="mt-5 text-2xl font-semibold tracking-tight text-zinc-100">LogHill</h1>
          <p className="mt-2 max-w-sm text-sm text-zinc-500">
            Observability center. Sign in with the password configured in the environment to continue.
          </p>
        </div>

        <form
          onSubmit={(event) => void onSubmit(event)}
          className="rounded-xl border border-zinc-800 bg-[#161618] p-6"
        >
          <label className="block text-xs font-medium text-zinc-300" htmlFor="app-password">
            Access password
          </label>
          <div className="relative mt-2">
            <Lock className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-zinc-600" />
            <Input
              id="app-password"
              autoFocus
              type={showPassword ? "text" : "password"}
              autoComplete="current-password"
              value={password}
              disabled={submitting}
              onChange={(change) => {
                setPassword(change.target.value);
                setError("");
              }}
              placeholder="Enter the password"
              className="w-full pl-10 pr-11"
              aria-invalid={Boolean(error) || undefined}
            />
            <button
              type="button"
              aria-label={showPassword ? "Hide password" : "Show password"}
              className="absolute right-1.5 top-1/2 grid size-8 -translate-y-1/2 place-items-center rounded-md text-zinc-500 transition-colors duration-150 hover:bg-zinc-800 hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50"
              onClick={() => setShowPassword((current) => !current)}
            >
              {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            </button>
          </div>

          {error && (
            <p role="alert" className="mt-3 rounded-lg border border-red-950 bg-red-950/20 px-3 py-2 text-xs text-red-300">
              {error}
            </p>
          )}

          <Button type="submit" disabled={submitting || !password.trim()} className="mt-5 h-10 w-full text-white">
            <LogIn className="size-4" />
            {submitting ? "Signing in..." : "Sign in"}
          </Button>
        </form>
      </div>
    </div>
  );
}
