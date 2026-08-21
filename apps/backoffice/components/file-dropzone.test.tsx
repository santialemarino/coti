import { fireEvent, render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { FileDropzone } from '@/components/file-dropzone';

describe('FileDropzone', () => {
  it('announces the active drop target and passes the dropped file to the caller', () => {
    const onFile = vi.fn();
    const view = render(
      <FileDropzone accept=".csv" onFile={onFile}>
        {({ dragging }) => <span>{dragging ? 'Release' : 'Drag'}</span>}
      </FileDropzone>,
    );
    const surface = view.getByText('Drag').parentElement!;
    const file = new File(['code,name'], 'catalog.csv', { type: 'text/csv' });

    fireEvent.dragEnter(surface);
    expect(view.getByText('Release')).toBeTruthy();
    expect(surface.getAttribute('data-dragging')).toBe('true');

    fireEvent.drop(surface, { dataTransfer: { files: [file] } });
    expect(onFile).toHaveBeenCalledWith(file);
    expect(view.getByText('Drag')).toBeTruthy();
    expect(surface.hasAttribute('data-dragging')).toBe(false);
  });
});
