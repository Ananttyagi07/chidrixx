// The real chidrixx brand mark (a provided asset, `public/chidrixx-mark.png`
// -- a 3D chrome "C", real transparency around it). Reused everywhere the
// mark appears (landing page, the post-sign-in Top HUD) so it only ever
// needs updating in one place. `className` sets the bounding box; the
// image itself scales to fit it via `object-contain`, since the source
// asset's real aspect ratio isn't square.
export function ChidrixxMark({ className = "" }: { className?: string }) {
  return (
    <span className={`inline-flex items-center justify-center ${className}`}>
      <img src="/chidrixx-mark.png" alt="chidrixx" className="h-full w-full object-contain" />
    </span>
  );
}
