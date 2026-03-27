#!/usr/bin/env node
// Converts TypeScript source to simplified JSON AST for the Go transpiler.
// Usage: node ts_ast.js < input.ts > output.json

const ts = require("typescript");
const fs = require("fs");

const source = fs.readFileSync(process.argv[2] || "/dev/stdin", "utf-8");
const sf = ts.createSourceFile("input.ts", source, ts.ScriptTarget.Latest, true);

function convertNode(node) {
  const kind = ts.SyntaxKind[node.kind];
  const result = { kind };

  switch (node.kind) {
    case ts.SyntaxKind.SourceFile:
      result.children = node.statements.map(convertNode);
      break;

    case ts.SyntaxKind.ClassDeclaration:
      result.name = node.name?.text || "";
      result.children = node.members.map(convertNode);
      break;

    case ts.SyntaxKind.PropertyDeclaration:
      result.name = node.name?.text || "";
      result.type = typeToString(node.type);
      result.visibility = getVisibility(node);
      if (node.initializer) result.init = convertNode(node.initializer);
      break;

    case ts.SyntaxKind.Constructor:
      result.kind = "Constructor";
      result.params = (node.parameters || []).map(convertParam);
      result.body = node.body ? node.body.statements.map(convertNode) : [];
      break;

    case ts.SyntaxKind.MethodDeclaration:
      result.name = node.name?.text || "";
      result.params = (node.parameters || []).map(convertParam);
      result.returnType = typeToString(node.type);
      result.visibility = getVisibility(node);
      result.body = node.body ? node.body.statements.map(convertNode) : [];
      break;

    // Statements
    case ts.SyntaxKind.VariableStatement:
      const decl = node.declarationList.declarations[0];
      result.kind = "VariableDeclaration";
      result.name = decl.name.text;
      result.type = typeToString(decl.type);
      if (decl.initializer) result.init = convertNode(decl.initializer);
      break;

    case ts.SyntaxKind.ExpressionStatement:
      result.expression = convertNode(node.expression);
      // Check for assignment: x = y
      if (node.expression.kind === ts.SyntaxKind.BinaryExpression &&
          node.expression.operatorToken.kind === ts.SyntaxKind.EqualsToken) {
        result.kind = "Assignment";
        result.left = convertNode(node.expression.left);
        result.right = convertNode(node.expression.right);
      }
      break;

    case ts.SyntaxKind.ReturnStatement:
      result.kind = "ReturnStatement";
      if (node.expression) result.expression = convertNode(node.expression);
      break;

    case ts.SyntaxKind.IfStatement:
      result.kind = "IfStatement";
      result.condition = convertNode(node.expression);
      result.then = convertBlock(node.thenStatement);
      if (node.elseStatement) {
        if (node.elseStatement.kind === ts.SyntaxKind.IfStatement) {
          result.else = convertNode(node.elseStatement);
        } else {
          result.else = convertBlock(node.elseStatement);
        }
      }
      break;

    case ts.SyntaxKind.WhileStatement:
      result.kind = "WhileStatement";
      result.condition = convertNode(node.expression);
      result.body = convertBlock(node.statement).statements;
      break;

    case ts.SyntaxKind.ForStatement:
      result.kind = "ForStatement";
      if (node.initializer) {
        if (node.initializer.kind === ts.SyntaxKind.VariableDeclarationList) {
          const d = node.initializer.declarations[0];
          result.init = { kind: "VariableDeclaration", name: d.name.text, type: typeToString(d.type) };
          if (d.initializer) result.init.init = convertNode(d.initializer);
        } else {
          result.init = convertNode(node.initializer);
        }
      }
      if (node.condition) result.condition = convertNode(node.condition);
      if (node.incrementor) result.increment = convertExprAsStmt(node.incrementor);
      result.body = convertBlock(node.statement).statements;
      break;

    case ts.SyntaxKind.ContinueStatement:
      result.kind = "ContinueStatement";
      break;

    case ts.SyntaxKind.BreakStatement:
      result.kind = "BreakStatement";
      break;

    case ts.SyntaxKind.Block:
      result.kind = "Block";
      result.statements = node.statements.map(convertNode);
      break;

    // Expressions
    case ts.SyntaxKind.NumericLiteral:
      result.kind = "NumericLiteral";
      result.value = node.text;
      break;

    case ts.SyntaxKind.StringLiteral:
      result.kind = "StringLiteral";
      result.value = node.text;
      break;

    case ts.SyntaxKind.TrueKeyword:
      result.kind = "BooleanLiteral";
      result.value = "true";
      break;
    case ts.SyntaxKind.FalseKeyword:
      result.kind = "BooleanLiteral";
      result.value = "false";
      break;
    case ts.SyntaxKind.NullKeyword:
      result.kind = "NullLiteral";
      break;

    case ts.SyntaxKind.Identifier:
      result.kind = "Identifier";
      result.name = node.text;
      break;

    case ts.SyntaxKind.ThisKeyword:
      result.kind = "ThisExpression";
      break;

    case ts.SyntaxKind.BinaryExpression:
      result.kind = "BinaryExpression";
      result.left = convertNode(node.left);
      result.right = convertNode(node.right);
      result.operator = ts.tokenToString(node.operatorToken.kind);
      break;

    case ts.SyntaxKind.PrefixUnaryExpression:
    case ts.SyntaxKind.PostfixUnaryExpression:
      result.kind = "UnaryExpression";
      result.operator = ts.tokenToString(node.operator);
      result.expression = convertNode(node.operand);
      break;

    case ts.SyntaxKind.PropertyAccessExpression:
      if (node.expression.kind === ts.SyntaxKind.ThisKeyword) {
        result.kind = "PropertyAccess";
        result.object = { kind: "ThisExpression" };
        result.property = node.name.text;
      } else {
        result.kind = "PropertyAccess";
        result.object = convertNode(node.expression);
        result.property = node.name.text;
      }
      break;

    case ts.SyntaxKind.CallExpression:
      if (node.expression.kind === ts.SyntaxKind.PropertyAccessExpression) {
        result.kind = "MethodCall";
        result.object = convertNode(node.expression.expression);
        result.property = node.expression.name.text;
      } else {
        result.kind = "MethodCall";
        result.object = convertNode(node.expression);
        result.property = "";
      }
      result.arguments = node.arguments.map(convertNode);
      break;

    case ts.SyntaxKind.NewExpression:
      result.kind = "NewExpression";
      result.name = node.expression.text || "";
      result.arguments = (node.arguments || []).map(convertNode);
      break;

    case ts.SyntaxKind.ArrayLiteralExpression:
      result.kind = "ArrayLiteral";
      result.children = node.elements.map(convertNode);
      break;

    case ts.SyntaxKind.ConditionalExpression:
      result.kind = "ConditionalExpression";
      result.condition = convertNode(node.condition);
      result.then = convertNode(node.whenTrue);
      result.else = convertNode(node.whenFalse);
      break;

    case ts.SyntaxKind.ParenthesizedExpression:
      return convertNode(node.expression);

    default:
      result.text = node.getText(sf).substring(0, 100);
  }

  return result;
}

function convertParam(param) {
  return {
    kind: "Parameter",
    name: param.name.text,
    type: typeToString(param.type),
  };
}

function convertBlock(stmt) {
  if (stmt.kind === ts.SyntaxKind.Block) {
    return { kind: "Block", statements: stmt.statements.map(convertNode) };
  }
  return { kind: "Block", statements: [convertNode(stmt)] };
}

function convertExprAsStmt(expr) {
  const node = convertNode(expr);
  if (node.kind === "UnaryExpression" && (node.operator === "++" || node.operator === "--")) {
    return {
      kind: "Assignment",
      left: node.expression,
      right: {
        kind: "BinaryExpression",
        left: node.expression,
        right: { kind: "NumericLiteral", value: "1" },
        operator: node.operator === "++" ? "+" : "-",
      },
    };
  }
  return { kind: "ExpressionStatement", expression: node };
}

function typeToString(typeNode) {
  if (!typeNode) return "";
  if (typeNode.kind === ts.SyntaxKind.NumberKeyword) return "number";
  if (typeNode.kind === ts.SyntaxKind.StringKeyword) return "string";
  if (typeNode.kind === ts.SyntaxKind.BooleanKeyword) return "boolean";
  if (typeNode.kind === ts.SyntaxKind.VoidKeyword) return "void";
  if (typeNode.kind === ts.SyntaxKind.ArrayType) return typeToString(typeNode.elementType) + "[]";
  if (typeNode.kind === ts.SyntaxKind.TypeReference) return typeNode.typeName?.text || "";
  return typeNode.getText(sf);
}

function getVisibility(node) {
  if (node.modifiers) {
    for (const mod of node.modifiers) {
      if (mod.kind === ts.SyntaxKind.PrivateKeyword) return "private";
      if (mod.kind === ts.SyntaxKind.ProtectedKeyword) return "protected";
      if (mod.kind === ts.SyntaxKind.PublicKeyword) return "public";
    }
  }
  return "public";
}

const ast = convertNode(sf);
console.log(JSON.stringify(ast, null, 2));
