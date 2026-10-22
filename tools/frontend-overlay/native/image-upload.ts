export const UI_IMAGE_MAX_BYTES = 2 * 1024 * 1024
export const UI_IMAGE_MAX_DIMENSION = 4096
export const UI_IMAGE_MAX_PIXELS = 4 * 1024 * 1024
export function imageUploadProblem(file: Pick<File, 'type' | 'size'>): string | undefined {
  if (!['image/png', 'image/jpeg', 'image/webp'].includes(file.type)) return '请选择 PNG、JPEG 或 WebP 图片。'
  if (file.size <= 0 || file.size > UI_IMAGE_MAX_BYTES) return '请选择不超过 2 MiB 的有效图片。'
}
export function normalizedImageSize(width: number, height: number): { width: number; height: number } {
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0 || width > UI_IMAGE_MAX_DIMENSION || height > UI_IMAGE_MAX_DIMENSION || width * height > UI_IMAGE_MAX_PIXELS) throw new Error('图片尺寸过大，请使用边长不超过 4096 像素、总像素不超过 400 万的图片。')
  const scale = Math.min(1, 512 / Math.max(width, height))
  return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) }
}
// Re-encode through the browser pixel decoder; never upload SVG or a supplied URL.
export async function normalizeUIImage(file: File): Promise<Blob> {
  const problem = imageUploadProblem(file)
  if (problem) throw new Error(problem)
  const source = URL.createObjectURL(file), image = new Image()
  try {
    await new Promise<void>((resolve, reject) => { image.onload = () => resolve(); image.onerror = () => reject(new Error('无法读取图片，请选择有效的 PNG、JPEG 或 WebP 文件。')); image.src = source })
    const size = normalizedImageSize(image.naturalWidth, image.naturalHeight), canvas = document.createElement('canvas')
    canvas.width = size.width; canvas.height = size.height
    const context = canvas.getContext('2d')
    if (!context) throw new Error('浏览器无法处理图片，请更新浏览器后重试。')
    context.drawImage(image, 0, 0, size.width, size.height)
    const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob(value => value ? resolve(value) : reject(new Error('图片处理失败，请换一张图片。')), 'image/png'))
    if (blob.size > UI_IMAGE_MAX_BYTES) throw new Error('处理后的图片仍超过 2 MiB，请选择较小的图片。')
    return blob
  } finally { URL.revokeObjectURL(source) }
}
export async function normalizedImageDataURL(file: File): Promise<string> {
  const blob = await normalizeUIImage(file)
  return new Promise((resolve, reject) => { const reader = new FileReader(); reader.onload = () => resolve(String(reader.result)); reader.onerror = () => reject(new Error('图片读取失败。')); reader.readAsDataURL(blob) })
}
