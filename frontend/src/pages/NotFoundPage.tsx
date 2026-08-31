import { ArrowLeft } from "lucide-react";
import { Link } from "react-router-dom";
import { Panel } from "../components/ui";

export function NotFoundPage() {
  return (
    <Panel className="grid min-h-[420px] place-items-center p-6 text-center">
      <div>
        <p className="font-mono text-xs text-zinc-600">HTTP 404</p>
        <h2 className="mt-2 text-xl font-semibold">Page not found</h2>
        <p className="mt-2 text-sm text-zinc-500">
          The requested address does not exist in this application.
        </p>
        <Link
          to="/"
          className="mt-5 inline-flex h-9 items-center gap-2 rounded-lg border border-zinc-700 bg-zinc-900 px-3 text-sm text-zinc-200 hover:bg-zinc-800"
        >
          <ArrowLeft className="size-4" />
          Back to dashboard
        </Link>
      </div>
    </Panel>
  );
}
