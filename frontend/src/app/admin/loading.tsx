export default function AdminLoading() {
  return (
    <div className="space-y-4 p-6">
      <div className="h-8 w-56 animate-pulse rounded-lg bg-stone-200" />
      <div className="space-y-3">
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={i} className="h-12 animate-pulse rounded-lg bg-stone-100" />
        ))}
      </div>
    </div>
  );
}
