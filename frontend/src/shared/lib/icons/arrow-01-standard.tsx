import type {SVGProps} from "react"

type ArrowProps = Omit<SVGProps<SVGSVGElement>, "direction"> & {size?: number | string}
type Direction = "down" | "left" | "right" | "up"

// Simple, independently drawn standard-stroke chevrons. These are not
// Hugeicons Pro assets; those require a separate Stroke Standard license.
const paths: Record<Direction, string> = {
  down: "M6 9L12 15L18 9",
  left: "M15 6L9 12L15 18",
  right: "M9 6L15 12L9 18",
  up: "M6 15L12 9L18 15",
}

function Arrow01({
  direction,
  size = 24,
  width,
  height,
  strokeWidth = 1.5,
  ...props
}: ArrowProps & {direction: Direction}) {
  return (
    <svg
      width={width ?? size}
      height={height ?? size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="square"
      strokeLinejoin="miter"
      aria-hidden="true"
      xmlns="http://www.w3.org/2000/svg"
      {...props}
    >
      <path d={paths[direction]} />
    </svg>
  )
}

export const ArrowDown01StandardIcon = (props: ArrowProps) => <Arrow01 direction="down" {...props} />
export const ArrowLeft01StandardIcon = (props: ArrowProps) => <Arrow01 direction="left" {...props} />
export const ArrowRight01StandardIcon = (props: ArrowProps) => <Arrow01 direction="right" {...props} />
export const ArrowUp01StandardIcon = (props: ArrowProps) => <Arrow01 direction="up" {...props} />
