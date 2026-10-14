'use client';

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  type ForwardedRef,
  type KeyboardEvent,
} from 'react';

// A pause this long between keystrokes closes a run of typing into its own undo step.
const UNDO_GROUP_PAUSE_MS = 1000;
const MAX_UNDO_STEPS = 100;
const NAVIGATION_KEYS = new Set(['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End']);

export interface InputSnapshot {
  value: string;
  start: number;
  end: number;
}

// A run of `insert` or of `delete` edits is one undo step; any other edit is a step of its own.
export type EditKind = 'insert' | 'delete' | 'other';

interface LastEdit {
  kind: EditKind;
  caret: number;
  at: number;
}

interface InputHistoryOptions {
  ref: ForwardedRef<HTMLInputElement>;
  // What the field shows now; a change that no recorded edit produced resets the history.
  value: string;
  restore: (snapshot: InputSnapshot, input: HTMLInputElement) => void;
  onReset?: () => void;
}

export function editKind(event: Event): EditKind {
  const inputType = 'inputType' in event ? event.inputType : undefined;
  if (inputType === 'insertText') return 'insert';
  if (inputType === 'deleteContentBackward' || inputType === 'deleteContentForward') {
    return 'delete';
  }
  return 'other';
}

export function snapshotOf(input: HTMLInputElement): InputSnapshot {
  const end = input.selectionEnd ?? input.value.length;
  return { value: input.value, start: input.selectionStart ?? end, end };
}

function shortcutOf(event: KeyboardEvent<HTMLInputElement>): 'undo' | 'redo' | null {
  if (!(event.metaKey || event.ctrlKey) || event.altKey) return null;
  const key = event.key.toLowerCase();
  if (key === 'z') return event.shiftKey ? 'redo' : 'undo';
  if (key === 'y' && event.ctrlKey && !event.shiftKey) return 'redo';
  return null;
}

// Undo for a field that rewrites its own value, which the browser leaves out of its own history.
export function useInputHistory({ ref, value, restore, onReset }: InputHistoryOptions) {
  const undoStack = useRef<InputSnapshot[]>([]);
  const redoStack = useRef<InputSnapshot[]>([]);
  const lastEdit = useRef<LastEdit | null>(null);
  const pending = useRef<InputSnapshot | null>(null);
  const known = useRef(value);
  const menuStepTaken = useRef(false);
  const restoreLatest = useRef(restore);
  const onResetLatest = useRef(onReset);

  useLayoutEffect(() => {
    restoreLatest.current = restore;
    onResetLatest.current = onReset;
  });

  useEffect(() => {
    if (value === known.current) return;
    undoStack.current = [];
    redoStack.current = [];
    lastEdit.current = null;
    known.current = value;
    onResetLatest.current?.();
  }, [value]);

  const step = useCallback((input: HTMLInputElement, direction: 'undo' | 'redo') => {
    const [from, to] = direction === 'undo' ? [undoStack, redoStack] : [redoStack, undoStack];
    const snapshot = from.current.pop();
    if (!snapshot) return;
    to.current.push(snapshotOf(input));
    lastEdit.current = null;
    known.current = snapshot.value;
    restoreLatest.current(snapshot, input);
  }, []);

  const inputRef = useCallback(
    (input: HTMLInputElement | null) => {
      if (typeof ref === 'function') ref(input);
      else if (ref) ref.current = input;
      if (!input) return;

      // The Edit menu and the context menu undo through `beforeinput`, never through a key. WebKit
      // answers one such undo with an event per entry of its own stack, all in one task.
      const onBeforeInput = (event: InputEvent) => {
        if (event.inputType === 'historyUndo' || event.inputType === 'historyRedo') {
          event.preventDefault();
          if (menuStepTaken.current) return;
          menuStepTaken.current = true;
          setTimeout(() => {
            menuStepTaken.current = false;
          });
          step(input, event.inputType === 'historyUndo' ? 'undo' : 'redo');
          return;
        }
        pending.current = snapshotOf(input);
      };
      // A click may put the caret back where the run left it, which must still close the run.
      const onPointerDown = () => {
        lastEdit.current = null;
      };
      input.addEventListener('beforeinput', onBeforeInput);
      input.addEventListener('pointerdown', onPointerDown);
      return () => {
        input.removeEventListener('beforeinput', onBeforeInput);
        input.removeEventListener('pointerdown', onPointerDown);
        if (typeof ref === 'function') ref(null);
        else if (ref) ref.current = null;
      };
    },
    [ref, step],
  );

  // The field as it stood before the edit in progress: `beforeinput` saw it, unless the edit
  // (autofill) skipped that event, and then the caret is taken to have been at the end.
  function before(): InputSnapshot {
    const snapshot = pending.current ?? { value, start: value.length, end: value.length };
    pending.current = null;
    return snapshot;
  }

  function record(kind: EditKind, from: InputSnapshot, to: InputSnapshot) {
    const last = lastEdit.current;
    if (from.value === to.value) {
      if (last?.kind === kind) last.caret = to.end;
      return;
    }
    const now = Date.now();
    const continues =
      kind !== 'other' &&
      last?.kind === kind &&
      from.start === from.end &&
      from.start === last.caret &&
      now - last.at < UNDO_GROUP_PAUSE_MS;
    if (!continues) undoStack.current = [...undoStack.current, from].slice(-MAX_UNDO_STEPS);
    redoStack.current = [];
    lastEdit.current = kind === 'other' ? null : { kind, caret: to.end, at: now };
    known.current = to.value;
  }

  // True when the key was an undo or redo shortcut, which the field has then handled.
  function onKeyDown(event: KeyboardEvent<HTMLInputElement>): boolean {
    if (NAVIGATION_KEYS.has(event.key)) lastEdit.current = null;
    const shortcut = shortcutOf(event);
    if (!shortcut || event.currentTarget.readOnly) return false;
    event.preventDefault();
    step(event.currentTarget, shortcut);
    return true;
  }

  return { inputRef, before, record, onKeyDown };
}
