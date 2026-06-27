import { PALETTE } from '../types';

interface Props {
  selected: string;
  onSelect: (color: string) => void;
}

export function ColorPalette({ selected, onSelect }: Props) {
  return (
    <div className="palette">
      {PALETTE.map((color) => (
        <button
          key={color}
          className={`swatch ${color === selected ? 'selected' : ''}`}
          style={{ backgroundColor: color }}
          onClick={() => onSelect(color)}
          aria-label={color}
        />
      ))}
    </div>
  );
}
