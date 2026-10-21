import type { SVGProps } from "react";

type Props = SVGProps<SVGSVGElement> & { size?: number | string };

// Source: Hugeicons Free Stroke Rounded (MIT), https://hugeicons.com/icons
export const Cancel01StrokeRoundedIcon = ({ size = 24, width, height, color = "currentColor", strokeWidth = 1.5, ...props }: Props) => (
  <svg
    width={width ?? size}
    height={height ?? size}
    viewBox="0 0 24 24"
    fill="none"
    color={color}
    strokeWidth={strokeWidth}
    xmlns="http://www.w3.org/2000/svg"
    {...props}
  >
    <path d="M18 6L6.00081 17.9992M17.9992 18L6 6.00085" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round"/>
  </svg>
);
