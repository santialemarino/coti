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
