export function AppFooter() {
  return (
    <footer className="flex h-24 w-full shrink-0 flex-col items-center justify-center gap-2 border-t border-zinc-800 bg-[#111113] px-4 py-3 text-xs text-zinc-500 sm:h-16 sm:flex-row sm:justify-between sm:px-6">
      <div className="flex min-w-0 items-center gap-3">
        <img src="/loghill.png" alt="LogHill" className="size-12 object-contain" />
        <span className="truncate font-medium text-zinc-300">Powered by LogHill</span>
      </div>
      <div className="flex min-w-0 items-center gap-2">
        <span className="truncate">Criado por Pedro Borges</span>
        <span aria-hidden="true" className="text-zinc-700">|</span>
        <span className="whitespace-nowrap">CPAL 1.0</span>
      </div>
    </footer>
  );
}
