'use client';

import * as React from 'react';
import { UploadCloudIcon, type LucideIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { buttonVariants } from './button';
import { Spinner } from './spinner';

interface DropzoneProps {
  /* Mirrors the file input's `accept`; the browser filter and the copy must agree. */
  accept: string;
  onFile: (file: File | undefined) => void;
  /* The resting invitation. Replaced by the file's name once one is chosen. */
  title: string;
  /* Shown while a file is over the surface. */
  releaseLabel: string;
  chooseLabel: string;
  /* Which formats are allowed, in words. */
  hint?: string;
  /* The chosen file's name and a short detail under it (a size, a sheet count). */
  fileName?: string | null;
  fileMeta?: string | null;
  icon?: LucideIcon;
  /* Replaces the icon once there is something to show — an image preview, a file glyph. */
  preview?: React.ReactNode;
  disabled?: boolean;
  /* Swaps the icon for a spinner and locks the surface, for an upload already in flight. */
  loading?: boolean;
  loadingLabel?: string;
  className?: string;
}

/*
 * The one file-intake surface: the onboarding logo, the catalog import, the price list. The whole
 * dashed box is the control, so there is a single hit target and a single focus ring — the inner
 * "elegir archivo" is a span wearing the outline button's clothes, because a real button inside a
 * button is invalid markup and the browser drops one of them.
 *
 * Anything the surface cannot own (removing what was chosen) belongs under the box, not inside it,
 * for the same reason.
 *
 * The drag state is counted rather than toggled: `dragleave` fires every time the pointer crosses
 * into a child element, so a boolean flickers off over the icon and the label.
 */
function Dropzone({
  accept,
  onFile,
  title,
  releaseLabel,
  chooseLabel,
  hint,
  fileName,
  fileMeta,
  icon: Icon = UploadCloudIcon,
  preview,
  disabled = false,
  loading = false,
  loadingLabel,
  className,
}: DropzoneProps) {
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [dragDepth, setDragDepth] = React.useState(0);
  const dragging = dragDepth > 0 && !disabled && !loading;
  const locked = disabled || loading;

  function openFileDialog() {
    if (!inputRef.current) return;
    /* Clearing first is what lets the same file be re-picked after a failed import. */
    inputRef.current.value = '';
    inputRef.current.click();
  }

  return (
    /*
     * The input is the button's sibling, not its child. A `<button>` may not contain interactive
     * content, and a file input is interactive — the same rule that keeps the "elegir archivo"
     * affordance a span. Off-screen rather than `display: none`, so a form that submits the element
     * still finds it.
     */
    <div className="contents">
      <input
        ref={inputRef}
        type="file"
        accept={accept}
        disabled={locked}
        className="sr-only"
        onChange={(event) => onFile(event.target.files?.[0])}
      />
      <button
        type="button"
        data-slot="dropzone"
        data-dragging={dragging || undefined}
        disabled={locked}
        onClick={openFileDialog}
        onDragEnter={(event) => {
          event.preventDefault();
          if (!locked) setDragDepth((depth) => depth + 1);
        }}
        onDragLeave={(event) => {
          event.preventDefault();
          setDragDepth((depth) => Math.max(0, depth - 1));
        }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          event.preventDefault();
          setDragDepth(0);
          if (!locked) onFile(event.dataTransfer.files[0]);
        }}
        className={cn(
          'group/dropzone flex flex-col w-full items-center justify-center px-6 py-10 gap-y-3',
          'bg-card border-2 border-dashed border-border rounded-1.5xl outline-none',
          'transition-[background-color,border-color,box-shadow] duration-200 ease-out-soft',
          'hover:border-border-strong hover:bg-muted',
          'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
          'active:bg-surface-hover',
          'data-[dragging]:border-primary data-[dragging]:bg-accent data-[dragging]:shadow-e3',
          'disabled:pointer-events-none disabled:opacity-60',
          className,
        )}
      >
        {preview ?? (
          <span
            aria-hidden="true"
            className={cn(
              'grid size-12 shrink-0 place-items-center bg-accent rounded-full text-accent-foreground',
              'transition-[scale,translate,background-color] duration-200 ease-out-soft',
              'group-hover/dropzone:bg-accent-strong',
              'group-data-[dragging]/dropzone:scale-110 group-data-[dragging]/dropzone:-translate-y-1',
            )}
          >
            {loading ? <Spinner size="sm" /> : <Icon className="size-6" />}
          </span>
        )}

        <span className="flex flex-col items-center gap-y-1 text-center">
          <span className="break-all text-paragraph-medium text-foreground">
            {loading ? (loadingLabel ?? title) : dragging ? releaseLabel : (fileName ?? title)}
          </span>
          {fileMeta ? (
            <span className="text-paragraph-sm text-foreground-muted">{fileMeta}</span>
          ) : hint ? (
            <span className="text-paragraph-xs text-foreground-muted">{hint}</span>
          ) : null}
        </span>

        <span
          aria-hidden="true"
          className={cn(
            buttonVariants({ variant: 'outline', size: 'default' }),
            'pointer-events-none group-hover/dropzone:border-border-strong group-hover/dropzone:bg-background',
          )}
        >
          {chooseLabel}
        </span>
      </button>
    </div>
  );
}

export { Dropzone };
