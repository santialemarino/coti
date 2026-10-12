'use client';

import { useEffect, useRef, useState } from 'react';
import { useTranslations } from 'next-intl';

import {
  Button,
  Callout,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  PendingButton,
} from '@repo/ui/components';
import { logoCropGeometry } from '@/lib/utils/logo-crop';

interface LogoCropDialogProps {
  file: File;
  onCancel: () => void;
  onConfirm: (file: File) => void;
}

export function LogoCropDialog({ file, onCancel, onConfirm }: LogoCropDialogProps) {
  const t = useTranslations('common.logoUpload.crop');
  const canvas = useRef<HTMLCanvasElement>(null);
  const drag = useRef<{ x: number; y: number; offsetX: number; offsetY: number } | null>(null);
  const [image, setImage] = useState<HTMLImageElement | null>(null);
  const [shape, setShape] = useState<'original' | 'square' | 'wide'>('original');
  const [zoom, setZoom] = useState(1);
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [pending, setPending] = useState(false);
  const [error, setError] = useState(false);
  const ratio =
    shape === 'original'
      ? image
        ? image.naturalWidth / image.naturalHeight
        : 3
      : shape === 'square'
        ? 1
        : 3;
  const width = Math.max(1, Math.round(Math.min(600, 280 * ratio)));
  const height = Math.max(1, Math.round(width / ratio));

  useEffect(() => {
    const url = URL.createObjectURL(file);
    const source = new window.Image();
    let active = true;
    source.onload = () => {
      if (active) setImage(source);
    };
    source.onerror = () => {
      if (active) setError(true);
    };
    source.src = url;
    return () => {
      active = false;
      URL.revokeObjectURL(url);
    };
  }, [file]);

  useEffect(() => {
    const context = canvas.current?.getContext('2d');
    if (!context || !image) return;
    const geometry = logoCropGeometry(
      image.naturalWidth,
      image.naturalHeight,
      width,
      height,
      zoom,
      offset.x,
      offset.y,
    );
    context.clearRect(0, 0, width, height);
    context.drawImage(image, geometry.x, geometry.y, geometry.width, geometry.height);
  }, [image, width, height, zoom, offset]);

  function reset(nextShape = shape) {
    setError(false);
    setShape(nextShape);
    setZoom(1);
    setOffset({ x: 0, y: 0 });
  }

  async function confirm() {
    if (!image) return;
    setPending(true);
    setError(false);
    try {
      const output = document.createElement('canvas');
      const outputWidth = Math.max(1, Math.round(Math.min(1200, 1200 * ratio)));
      const outputHeight = Math.max(1, Math.round(outputWidth / ratio));
      output.width = outputWidth;
      output.height = outputHeight;
      const context = output.getContext('2d');
      if (!context) throw new Error('Canvas unavailable');
      const geometry = logoCropGeometry(
        image.naturalWidth,
        image.naturalHeight,
        outputWidth,
        outputHeight,
        zoom,
        offset.x,
        offset.y,
      );
      context.drawImage(image, geometry.x, geometry.y, geometry.width, geometry.height);
      const blob = await new Promise<Blob>((resolve, reject) =>
        output.toBlob(
          (value) => (value ? resolve(value) : reject(new Error('Logo export failed'))),
          'image/png',
        ),
      );
      onConfirm(
        new File([blob], `${file.name.replace(/\.[^.]+$/, '')}.png`, { type: 'image/png' }),
      );
    } catch {
      setError(true);
    } finally {
      setPending(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !pending && onCancel()}>
      <DialogContent className="sm:max-w-lg" closeOnClickOutside={!pending}>
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
          <DialogDescription>{t('description')}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-wrap gap-2" role="group" aria-label={t('shape')}>
          {(['original', 'square', 'wide'] as const).map((value) => (
            <Button
              key={value}
              type="button"
              variant={shape === value ? 'secondary' : 'outline'}
              size="sm"
              aria-pressed={shape === value}
              disabled={pending}
              onClick={() => reset(value)}
            >
              {t(value)}
            </Button>
          ))}
        </div>
        <div className="flex min-h-48 items-center justify-center rounded-lg border border-border bg-sunken p-3">
          <canvas
            ref={canvas}
            width={width}
            height={height}
            role="img"
            aria-label={t('preview')}
            tabIndex={0}
            className="max-w-full touch-none cursor-grab rounded-sm bg-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing"
            onPointerDown={(event) => {
              if (!image || pending) return;
              event.currentTarget.setPointerCapture(event.pointerId);
              drag.current = {
                x: event.clientX,
                y: event.clientY,
                offsetX: offset.x,
                offsetY: offset.y,
              };
            }}
            onPointerMove={(event) => {
              if (!drag.current) return;
              const rect = event.currentTarget.getBoundingClientRect();
              setOffset({
                x: drag.current.offsetX + (event.clientX - drag.current.x) / rect.width,
                y: drag.current.offsetY + (event.clientY - drag.current.y) / rect.height,
              });
            }}
            onPointerUp={() => {
              drag.current = null;
            }}
            onPointerCancel={() => {
              drag.current = null;
            }}
            onKeyDown={(event) => {
              const directions: Record<string, [number, number]> = {
                ArrowLeft: [-0.03, 0],
                ArrowRight: [0.03, 0],
                ArrowUp: [0, -0.03],
                ArrowDown: [0, 0.03],
              };
              const direction = directions[event.key];
              if (direction && !pending) {
                event.preventDefault();
                setOffset((current) => ({
                  x: current.x + direction[0],
                  y: current.y + direction[1],
                }));
              }
            }}
          />
        </div>
        <div className="flex flex-col gap-y-2">
          <div className="relative flex h-8 items-center">
            <div
              aria-hidden="true"
              className="pointer-events-none absolute inset-x-2 h-1.5 overflow-hidden rounded-full bg-border"
            >
              <div className="h-full bg-primary" style={{ width: `${((zoom - 1) / 3) * 100}%` }} />
            </div>
            <input
              id="logo-crop-zoom"
              aria-label={t('zoom')}
              type="range"
              min={1}
              max={4}
              step={0.05}
              value={zoom}
              disabled={!image || pending}
              onChange={(event) => setZoom(Number(event.target.value))}
              className="relative m-0 h-8 w-full cursor-pointer appearance-none rounded-full bg-transparent outline-none focus-visible:ring-3 focus-visible:ring-ring/45 disabled:cursor-not-allowed disabled:opacity-50 [&::-webkit-slider-runnable-track]:h-1.5 [&::-webkit-slider-runnable-track]:bg-transparent [&::-webkit-slider-thumb]:-mt-1.25 [&::-webkit-slider-thumb]:size-4 [&::-webkit-slider-thumb]:appearance-none [&::-webkit-slider-thumb]:rounded-full [&::-webkit-slider-thumb]:bg-primary hover:[&::-webkit-slider-thumb]:bg-primary-hover active:[&::-webkit-slider-thumb]:bg-primary-active [&::-moz-range-track]:h-1.5 [&::-moz-range-track]:bg-transparent [&::-moz-range-thumb]:size-4 [&::-moz-range-thumb]:rounded-full [&::-moz-range-thumb]:border-0 [&::-moz-range-thumb]:bg-primary hover:[&::-moz-range-thumb]:bg-primary-hover active:[&::-moz-range-thumb]:bg-primary-active"
            />
          </div>
          <p className="text-paragraph-xs text-foreground-muted">{t('moveHint')}</p>
        </div>
        {error ? <Callout tone="danger">{t('error')}</Callout> : null}
        <DialogFooter>
          <Button type="button" variant="ghost" disabled={pending} onClick={() => reset()}>
            {t('reset')}
          </Button>
          <Button type="button" variant="outline" disabled={pending} onClick={onCancel}>
            {t('cancel')}
          </Button>
          <PendingButton
            type="button"
            pending={pending}
            pendingLabel={t('saving')}
            disabled={!image}
            onClick={confirm}
          >
            {t('confirm')}
          </PendingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
