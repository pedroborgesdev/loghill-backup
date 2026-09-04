export function AppFooter() {
  return (
    <footer className="flex h-24 w-full shrink-0 flex-col items-center justify-center gap-2 border-t border-zinc-800 bg-[#111113] px-4 py-3 text-xs text-zinc-500 sm:h-16 sm:flex-row sm:justify-between sm:px-6">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <img src="/loghill-flat.png" alt="LogHill" className="size-12 object-contain opacity-50" />
        <span className="truncate font-medium text-zinc-400">Powered by LogHill</span>
      </div>
      <a
        href="https://github.com/loghill-oss"
        target="_blank"
        rel="noreferrer"
        className="order-3 text-center text-zinc-400 underline decoration-zinc-700 underline-offset-2 transition-colors hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-white/50 sm:order-none sm:flex-1"
      >
        Visit other LogHill projects
      </a>
      <div className="flex min-w-0 flex-1 items-center justify-end gap-2">
        <span className="truncate">Created by Pedro Borges</span>
        <span aria-hidden="true" className="text-zinc-700">|</span>
        <span className="whitespace-nowrap">CPAL 1.0</span>
      </div>
    </footer>
  );
}
