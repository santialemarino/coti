'use client';

import * as React from 'react';
import { CheckIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { Popover, PopoverContent, PopoverTrigger } from './popover';

/* How far one arrow press moves a channel, and how far Page/Shift moves it. */
const STEP = 1;
const BIG_STEP = 10;

export interface ColorPickerLabels {
  /* Names the trigger, which otherwise announces only a colour. */
  trigger: string;
  /* The saturation/brightness pad and the hue rail are two separate controls. */
  shade: string;
  hue: string;
  presets: string;
}

interface ColorPickerProps {
  /* Six hex digits, no leading `#`. Anything else is treated as unset and falls back. */
  value: string;
  onValueChange: (hex: string) => void;
  labels: ColorPickerLabels;
  /* Offered as one-click choices above the pad — the brand's own colours belong here. */
  presets?: readonly string[];
  disabled?: boolean;
  id?: string;
  className?: string;
}

interface Hsv {
  h: number;
  s: number;
  v: number;
}

const HEX = /^[0-9a-f]{6}$/i;

function clamp(value: number, min = 0, max = 100) {
  return Math.min(max, Math.max(min, value));
}

export function hexToHsv(hex: string): Hsv {
  const r = parseInt(hex.slice(0, 2), 16) / 255;
  const g = parseInt(hex.slice(2, 4), 16) / 255;
  const b = parseInt(hex.slice(4, 6), 16) / 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const delta = max - min;

  let h = 0;
  if (delta !== 0) {
    if (max === r) h = ((g - b) / delta) % 6;
    else if (max === g) h = (b - r) / delta + 2;
    else h = (r - g) / delta + 4;
  }
  h = Math.round(h * 60);
  if (h < 0) h += 360;

  return { h, s: max === 0 ? 0 : (delta / max) * 100, v: max * 100 };
}

export function hsvToHex({ h, s, v }: Hsv): string {
  const saturation = s / 100;
  const value = v / 100;
  const chroma = value * saturation;
  const x = chroma * (1 - Math.abs(((h / 60) % 2) - 1));
  const m = value - chroma;

  const sector = Math.floor(h / 60) % 6;
  const [r, g, b] = (
    [
      [chroma, x, 0],
      [x, chroma, 0],
      [0, chroma, x],
      [0, x, chroma],
      [x, 0, chroma],
      [chroma, 0, x],
    ] as const
  )[sector] ?? [0, 0, 0];

  return [r, g, b]
    .map((channel) =>
      Math.round((channel + m) * 255)
        .toString(16)
        .padStart(2, '0'),
    )
    .join('')
    .toUpperCase();
}

/*
 * The colour control. `<input type="color">` hands the choice to an operating-system dialog that
 * cannot be styled, translated or keyboard-described — so on the one screen where a corralón picks
 * the colour its quotes will carry, the app stops looking like the app.
 *
 * Two one-dimensional controls rather than a pad plus a rail would be simpler, but a hue rail and a
 * saturation/brightness pad is what people already know how to use. Both are focusable and both move
 * on the arrow keys, because a control reachable only by pointer is not finished — and the field
 * that owns this picker keeps its hex input, which stays the fastest way in for anyone who already
 * knows the value.
 */
function ColorPicker({
  value,
  onValueChange,
  labels,
  presets,
  disabled = false,
  id,
  className,
}: ColorPickerProps) {
  const [open, setOpen] = React.useState(false);
  const padRef = React.useRef<HTMLDivElement>(null);
  const hueRef = React.useRef<HTMLDivElement>(null);

  const valid = HEX.test(value);
  const hex = valid ? value.toUpperCase() : '000000';
  /*
   * Derived from the hex on every render rather than held in state. A stored HSV drifts out of sync
   * the moment the hex input is typed into, and hue is genuinely unrecoverable from a greyscale hex
   * — so the one place it has to persist is while the pointer is down, which `lastHue` covers.
   */
  const hsv = hexToHsv(hex);
  const lastHue = React.useRef(hsv.h);
  if (hsv.s > 0 && hsv.v > 0) lastHue.current = hsv.h;
  const hue = hsv.s === 0 ? lastHue.current : hsv.h;

  function commit(next: Partial<Hsv>) {
    onValueChange(hsvToHex({ h: hue, s: hsv.s, v: hsv.v, ...next }));
  }

  /* Pointer position → channel values, for both the pad and the rail. */
  function trackPointer(
    element: HTMLElement | null,
    event: React.PointerEvent,
    apply: (x: number, y: number) => void,
  ) {
    if (!element) return;
    element.setPointerCapture(event.pointerId);
    const move = (clientX: number, clientY: number) => {
      const rect = element.getBoundingClientRect();
      apply(
        clamp(((clientX - rect.left) / rect.width) * 100),
        clamp(((clientY - rect.top) / rect.height) * 100),
      );
    };
    move(event.clientX, event.clientY);
    element.onpointermove = (moveEvent) => {
      if (moveEvent.buttons === 0) return;
      move(moveEvent.clientX, moveEvent.clientY);
    };
    element.onpointerup = () => {
      element.onpointermove = null;
      element.onpointerup = null;
    };
  }

  function arrowStep(event: React.KeyboardEvent, apply: (dx: number, dy: number) => void) {
    const step = event.shiftKey ? BIG_STEP : STEP;
    const moves: Record<string, [number, number]> = {
      ArrowLeft: [-step, 0],
      ArrowRight: [step, 0],
      ArrowUp: [0, -step],
      ArrowDown: [0, step],
    };
    const move = moves[event.key];
    if (!move) return;
    event.preventDefault();
    apply(move[0], move[1]);
  }

  return (
    <Popover open={open} onOpenChange={(next) => !disabled && setOpen(next)}>
      <PopoverTrigger asChild>
        <button
          id={id}
          type="button"
          disabled={disabled}
          aria-label={labels.trigger}
          className={cn(
            'grid size-9 shrink-0 place-items-center p-1 bg-input border border-border rounded-lg shadow-e1 outline-none',
            'transition-[border-color,box-shadow] duration-200 ease-out-soft',
            'hover:border-border-strong active:bg-surface-hover',
            'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
            'disabled:pointer-events-none disabled:opacity-50',
            className,
          )}
        >
          {/* The swatch keeps a hairline so a near-white choice is still a visible square. */}
          <span
            className="size-full border border-border/60 rounded-md"
            style={{ backgroundColor: `#${hex}` }}
          />
        </button>
      </PopoverTrigger>

      <PopoverContent align="end" className="w-64 p-3">
        <div className="flex flex-col gap-y-3">
          <div
            ref={padRef}
            role="group"
            tabIndex={disabled ? -1 : 0}
            aria-label={labels.shade}
            onPointerDown={(event) =>
              trackPointer(padRef.current, event, (x, y) => commit({ s: x, v: 100 - y }))
            }
            onKeyDown={(event) =>
              arrowStep(event, (dx, dy) => commit({ s: clamp(hsv.s + dx), v: clamp(hsv.v - dy) }))
            }
            className={cn(
              'relative h-32 w-full rounded-lg outline-none cursor-crosshair touch-none',
              'focus-visible:ring-3 focus-visible:ring-ring/45',
            )}
            style={{
              backgroundColor: `hsl(${hue} 100% 50%)`,
              backgroundImage:
                'linear-gradient(to right, #fff, transparent), linear-gradient(to top, #000, transparent)',
            }}
          >
            <span
              aria-hidden="true"
              className="absolute size-3 -translate-x-1/2 -translate-y-1/2 border-2 border-white rounded-full shadow-e2"
              style={{
                left: `${hsv.s}%`,
                top: `${100 - hsv.v}%`,
                backgroundColor: `#${hex}`,
              }}
            />
          </div>

          <div
            ref={hueRef}
            role="slider"
            tabIndex={disabled ? -1 : 0}
            aria-label={labels.hue}
            aria-valuemin={0}
            aria-valuemax={359}
            aria-valuenow={Math.round(hue)}
            onPointerDown={(event) =>
              trackPointer(hueRef.current, event, (x) => commit({ h: (x / 100) * 359 }))
            }
            onKeyDown={(event) =>
              arrowStep(event, (dx) => commit({ h: (hue + dx * 3.59 + 359) % 359 }))
            }
            className={cn(
              'relative h-3 w-full rounded-full outline-none cursor-pointer touch-none',
              'focus-visible:ring-3 focus-visible:ring-ring/45',
            )}
            style={{
              backgroundImage:
                'linear-gradient(to right, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00)',
            }}
          >
            <span
              aria-hidden="true"
              className="absolute top-1/2 size-4 -translate-x-1/2 -translate-y-1/2 border-2 border-white rounded-full shadow-e2"
              style={{
                left: `${(hue / 359) * 100}%`,
                backgroundColor: `hsl(${hue} 100% 50%)`,
              }}
            />
          </div>

          {presets?.length ? (
            <div role="group" aria-label={labels.presets} className="flex flex-wrap gap-1.5">
              {presets.map((preset) => {
                const selected = preset.toUpperCase() === hex;
                return (
                  <button
                    key={preset}
                    type="button"
                    aria-label={`#${preset.toUpperCase()}`}
                    aria-pressed={selected}
                    onClick={() => onValueChange(preset.toUpperCase())}
                    className={cn(
                      'grid size-6 place-items-center border border-border/60 rounded-md outline-none',
                      'transition-[box-shadow,border-color] duration-150 ease-out-soft',
                      'hover:border-border-strong',
                      'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
                    )}
                    style={{ backgroundColor: `#${preset}` }}
                  >
                    <CheckIcon
                      aria-hidden="true"
                      className={cn(
                        'size-3.5 text-white mix-blend-difference transition-[opacity,scale] duration-150 ease-out-soft',
                        selected ? 'scale-100 opacity-100' : 'scale-50 opacity-0',
                      )}
                    />
                  </button>
                );
              })}
            </div>
          ) : null}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export { ColorPicker };
