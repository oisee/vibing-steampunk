class Position {
  private row: number;
  private col: number;
  constructor(row: number, col: number) {
    this.row = row;
    this.col = col;
  }
  getRow(): number { return this.row; }
  getCol(): number { return this.col; }
}

class Token {
  private pos: Position;
  private str: string;
  constructor(pos: Position, str: string) {
    this.pos = pos;
    this.str = str;
  }
  getStr(): string { return this.str; }
  getPos(): Position { return this.pos; }
}

class Lexer {
  private tokens: Token[] = [];

  run(source: string): Token[] {
    this.tokens = [];
    let pos: number = 0;
    const len: number = source.length;

    while (pos < len) {
      const ch: string = source.charAt(pos);

      if (ch === " " || ch === "\n" || ch === "\t") {
        pos = pos + 1;
        continue;
      }

      if (ch === "*" && pos === 0) {
        let end: number = source.indexOf("\n", pos);
        if (end === -1) { end = len; }
        const comment: string = source.substring(pos, end);
        this.tokens.push(new Token(new Position(1, pos), comment));
        pos = end;
        continue;
      }

      if (ch === "\"") {
        let end: number = source.indexOf("\n", pos);
        if (end === -1) { end = len; }
        const comment: string = source.substring(pos, end);
        this.tokens.push(new Token(new Position(1, pos), comment));
        pos = end;
        continue;
      }

      if (ch === "'") {
        let end: number = source.indexOf("'", pos + 1);
        if (end === -1) { end = len; }
        const str: string = source.substring(pos, end + 1);
        this.tokens.push(new Token(new Position(1, pos), str));
        pos = end + 1;
        continue;
      }

      if (ch === "." || ch === "," || ch === ":" || ch === "(" || ch === ")" || ch === "[" || ch === "]") {
        this.tokens.push(new Token(new Position(1, pos), ch));
        pos = pos + 1;
        continue;
      }

      let end: number = pos + 1;
      while (end < len) {
        const c: string = source.charAt(end);
        if (c === " " || c === "\n" || c === "\t" || c === "." || c === "," || c === ":" || c === "(" || c === ")" || c === "[" || c === "]") {
          break;
        }
        end = end + 1;
      }
      this.tokens.push(new Token(new Position(1, pos), source.substring(pos, end)));
      pos = end;
    }

    return this.tokens;
  }
}
