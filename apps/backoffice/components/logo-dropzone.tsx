'use client';

import { useEffect, useState } from 'react';
import Image from 'next/image';
import { ImageIcon, XIcon } from 'lucide-react';
import { useTranslations } from 'next-intl';

import { Button, Callout, Dropzone } from '@repo/ui/components';

const ACCEPTED_TYPES = ['image/png', 'image/jpeg'];

interface LogoDropzoneProps {
  initialUrl?: string | null;
  onFileChange?: (file: File | null) => void;
  onPreviewChange?: (url: string | null) => void;
}

export function LogoDropzone({ initialUrl, onFileChange, onPreviewChange }: LogoDropzoneProps) {
  const t = useTranslations('common.logoUpload');
  const [selection, setSelection] = useState<File | null>();
  const [previewUrl, setPreviewUrl] = useState<string | null>(initialUrl ?? null);
  const [error, setError] = useState<string | null>(null);
  const file = selection instanceof File ? selection : null;

  useEffect(() => {
    if (selection === undefined) {
      setPreviewUrl(initialUrl ?? null);
      return;
    }
    if (selection === null) {
      setPreviewUrl(null);
      return;
    }
    const url = URL.createObjectURL(selection);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [initialUrl, selection]);

  useEffect(() => onPreviewChange?.(previewUrl), [onPreviewChange, previewUrl]);

  function choose(next: File | undefined) {
    if (!next) return;
    if (!ACCEPTED_TYPES.includes(next.type)) {
      setError(t('invalidType'));
      return;
    }
    setError(null);
    setSelection(next);
    onFileChange?.(next);
  }

  function clear() {
    setSelection(null);
    setError(null);
    onFileChange?.(null);
  }

  return (
    <div className="flex flex-col gap-y-3">
      <Dropzone
        accept="image/png,image/jpeg"
        onFile={choose}
        icon={ImageIcon}
        title={t('title')}
        releaseLabel={t('release')}
        chooseLabel={previewUrl ? t('replace') : t('choose')}
        fileName={file?.name}
        preview={
          previewUrl ? (
            <span className="relative flex h-24 w-full max-w-64 items-center justify-center bg-background border border-border rounded-lg">
              <Image
                src={previewUrl}
                alt={t('previewAlt')}
                fill
                unoptimized
                className="p-3 object-contain"
              />
            </span>
          ) : undefined
        }
      />
      {/* Outside the box: the dropzone is one button, and a button cannot contain another. */}
      {previewUrl ? (
        <Button type="button" variant="ghost" size="sm" className="self-start" onClick={clear}>
          <XIcon aria-hidden="true" />
          {t('remove')}
        </Button>
      ) : null}
      {error ? <Callout tone="danger">{error}</Callout> : null}
    </div>
  );
}
