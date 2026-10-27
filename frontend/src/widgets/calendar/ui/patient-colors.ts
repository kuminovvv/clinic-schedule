import type {CSSProperties} from "react"

const patientHue = (patientNumber: number) => ((Math.max(1, patientNumber) - 1) * 137.508) % 360

export const getPatientColorStyle = (patientNumber: number): CSSProperties => {
    const hue = patientHue(patientNumber)
    return {
        backgroundColor: `hsl(${hue} 82% 94%)`,
    }
}

export const getPatientColorMarkerStyle = (patientNumber: number): CSSProperties => ({
    backgroundColor: `hsl(${patientHue(patientNumber)} 65% 40%)`,
})
