export function logoCropGeometry(
  imageWidth: number,
  imageHeight: number,
  frameWidth: number,
  frameHeight: number,
  zoom: number,
  offsetX: number,
  offsetY: number,
) {
  const scale = Math.min(frameWidth / imageWidth, frameHeight / imageHeight) * zoom;
  const width = imageWidth * scale;
  const height = imageHeight * scale;
  const maxX = Math.max(0, (width - frameWidth) / 2);
  const maxY = Math.max(0, (height - frameHeight) / 2);
  return {
    width,
    height,
    x: (frameWidth - width) / 2 + Math.max(-maxX, Math.min(maxX, offsetX * frameWidth)),
    y: (frameHeight - height) / 2 + Math.max(-maxY, Math.min(maxY, offsetY * frameHeight)),
  };
}

export interface LogoOffset {
  x: number;
  y: number;
}

/*
 * Keeps a pan offset, a fraction of the frame, within what the zoomed image can travel. Stored
 * clamped, so dragging past an edge and back moves the image at once instead of through a dead zone.
 */
export function clampLogoOffset(
  imageWidth: number,
  imageHeight: number,
  frameWidth: number,
  frameHeight: number,
  zoom: number,
  offset: LogoOffset,
): LogoOffset {
  const scale = Math.min(frameWidth / imageWidth, frameHeight / imageHeight) * zoom;
  const maxX = Math.max(0, (imageWidth * scale - frameWidth) / 2) / frameWidth;
  const maxY = Math.max(0, (imageHeight * scale - frameHeight) / 2) / frameHeight;
  return {
    x: Math.max(-maxX, Math.min(maxX, offset.x)),
    y: Math.max(-maxY, Math.min(maxY, offset.y)),
  };
}
