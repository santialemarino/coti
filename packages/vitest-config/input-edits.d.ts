export function beforeInput(field: HTMLInputElement, inputType: string): boolean;

export function browserEdit(
  field: HTMLInputElement,
  raw: string,
  options?: { caret?: number; inputType?: string | null; announced?: boolean },
): void;

export function typeText(field: HTMLInputElement, text: string): void;
