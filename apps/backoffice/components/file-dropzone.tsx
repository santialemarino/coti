'use client';

import { useRef, useState } from 'react';

import { cn } from '@repo/ui/lib';

interface FileDropzoneRenderProps {
  dragging: boolean;
  openFileDialog: () => void;
}

interface FileDropzoneProps {
  accept: string;
  disabled?: boolean;
  onFile: (file: File | undefined) => void;
  children: (props: FileDropzoneRenderProps) => React.ReactNode;
  className?: string;
}

export function FileDropzone({
  accept,
  disabled = false,
  onFile,
  children,
  className,
}: FileDropzoneProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragDepth, setDragDepth] = useState(0);
  const dragging = dragDepth > 0;

  function resetDrag() {
    setDragDepth(0);
  }

  return (
    <div
      data-dragging={dragging || undefined}
      className={cn(
        'flex flex-col items-center justify-center px-6 py-8 gap-y-4 bg-card border-2 border-dashed border-strong rounded-1.5xl shadow-e1',
        'transition-[background-color,border-color,box-shadow] duration-200 ease-out-soft',
        'data-[dragging]:bg-accent data-[dragging]:border-primary data-[dragging]:shadow-e3',
        className,
      )}
      onDragEnter={(event) => {
        event.preventDefault();
        if (!disabled) setDragDepth((depth) => depth + 1);
      }}
      onDragLeave={(event) => {
        event.preventDefault();
        setDragDepth((depth) => Math.max(0, depth - 1));
      }}
      onDragOver={(event) => event.preventDefault()}
      onDrop={(event) => {
        event.preventDefault();
        resetDrag();
        if (!disabled) onFile(event.dataTransfer.files[0]);
      }}
    >
      <input
        ref={inputRef}
        type="file"
        accept={accept}
        disabled={disabled}
        className="sr-only"
        onChange={(event) => onFile(event.target.files?.[0])}
      />
      {children({
        dragging,
        openFileDialog: () => {
          if (!inputRef.current) return;
          inputRef.current.value = '';
          inputRef.current.click();
        },
      })}
    </div>
  );
}
