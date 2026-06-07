import { Project, SyntaxKind, JsxElement, JsxFragment, ArrowFunction, VariableDeclaration } from "ts-morph";
import * as fs from 'fs';
import * as path from 'path';

const project = new Project();
project.addSourceFilesAtPaths("web/client/src/pages/V2RayClientPage.tsx");
const sourceFile = project.getSourceFileOrThrow("V2RayClientPage.tsx");

const componentDecl = sourceFile.getVariableDeclarationOrThrow("V2RayClientPage");
const arrowFunc = componentDecl.getInitializerIfKindOrThrow(SyntaxKind.ArrowFunction);
const body = arrowFunc.getBodyIfKindOrThrow(SyntaxKind.Block);

// Find all state variables
const vars = new Set<string>();
body.getVariableStatements().forEach(stmt => {
    stmt.getDeclarations().forEach(decl => {
        const nameNode = decl.getNameNode();
        if (nameNode.getKind() === SyntaxKind.ArrayBindingPattern) {
            nameNode.getElements().forEach(el => {
                if (el.getKind() === SyntaxKind.BindingElement) {
                    vars.add(el.getText());
                }
            });
        } else if (nameNode.getKind() === SyntaxKind.Identifier) {
            vars.add(nameNode.getText());
        }
    });
});

// Remove PAGE_LIMIT
vars.delete("PAGE_LIMIT");
const contextKeys = Array.from(vars);

// Create Context
const contextDir = "web/client/src/pages/v2ray-client/context";
fs.mkdirSync(contextDir, { recursive: true });
fs.writeFileSync(path.join(contextDir, "V2RayClientContext.ts"), `import { createContext, useContext } from 'react';
export const V2RayClientContext = createContext<any>(null);
export const useV2RayClientContext = () => {
  const ctx = useContext(V2RayClientContext);
  if (!ctx) throw new Error('Must be inside V2RayClientProvider');
  return ctx;
};
`);

// Analyze JSX Return statement
const returnStmt = body.getStatements().find(s => s.getKind() === SyntaxKind.ReturnStatement);
if (!returnStmt) throw new Error("No return statement");

// The return expression is typically a ParenthesizedExpression containing a JsxElement
let returnExpr = (returnStmt as any).getExpression();
if (returnExpr.getKind() === SyntaxKind.ParenthesizedExpression) {
    returnExpr = returnExpr.getExpression();
}

let topJsx = returnExpr;

// We will just do a simpler string replacement for the components based on the child elements of the top div
const jsxText = topJsx.getFullText();

// Because parsing deeply nested JSX and preserving formatting is complex even with ts-morph,
// we will just wrap the return expression with Context Provider inside the file.
// Wait, the user asked to break it into components.

