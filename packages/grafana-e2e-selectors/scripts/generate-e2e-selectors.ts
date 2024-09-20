import { readFileSync } from 'fs';
import { writeFile } from 'node:fs/promises';
import { resolve, join } from 'path';
import * as semver from 'semver';
import * as ts from 'typescript';

import { MIN_GRAFANA_VERSION } from '../src/selectors/constants';

const packageJson = JSON.parse(readFileSync(resolve(process.cwd(), 'package.json')).toString());
const version = packageJson.version.replace(/\-.*/, ''); // remove any pre-release tags. we may want to add support build number in the future though
const sourceDir = 'src/selectors';
const destDir = 'src/generated';
const fileNames = ['components.ts', 'pages.ts', 'apis.ts'];
const sourceFiles = fileNames.map((fileName) => {
  const buffer = readFileSync(resolve(join(process.cwd(), sourceDir, fileName)));
  // replace usage of [MIN_GRAFANA_VERSION] variable with the actual value
  const code = buffer.toString().replace(/\[MIN_GRAFANA_VERSION\]/g, `'${MIN_GRAFANA_VERSION}'`);
  return ts.createSourceFile(fileName, code, ts.ScriptTarget.ES2015, /*setParentNodes */ true);
});

const getSelectorValue = (
  properties: ts.NodeArray<ts.ObjectLiteralElementLike>,
  escapedText: string,
  sourceFileName: string
): ts.PropertyAssignment | undefined => {
  let current: ts.PropertyAssignment | undefined = undefined;
  for (const property of properties) {
    if (
      property.name &&
      ts.isStringLiteral(property.name) &&
      ts.isPropertyAssignment(property) &&
      semver.satisfies(version, `>=${property.name.text.replace(/'/g, '')}`)
    ) {
      if (!current) {
        current = property;
      } else if (semver.gt(property.name.text.replace(/'/g, ''), current.name.getText().replace(/'/g, ''))) {
        current = property;
      }
    }
  }

  if (!current) {
    // selector doesn't have a version. should throw an error and terminate compilation, but for now just log a error
    console.error(
      `${sourceFileName}: Could not resolve a value for selector '${escapedText}' using version '${version}'`
    );
  }

  return current;
};

const replaceVersions = (context: ts.TransformationContext) => (rootNode: ts.Node) => {
  const visit = (node: ts.Node): ts.Node => {
    if (ts.isImportDeclaration(node) && node.getFullText().includes('.gen')) {
      return node;
    }

    // remove all nodes that are not source files or variable statements
    if (!ts.isSourceFile(node) && ts.isSourceFile(node.parent) && !ts.isVariableStatement(node)) {
      return ts.factory.createEmptyStatement();
    }

    const newNode = ts.visitEachChild(node, visit, context);

    if (ts.isObjectLiteralExpression(newNode) && newNode.parent) {
      const parentText = newNode.parent.getFirstToken()?.getText() || '';
      const propertyAssignment = getSelectorValue(newNode.properties, parentText, rootNode.getSourceFile().fileName);
      if (!propertyAssignment || !ts.isStringLiteral(propertyAssignment.name)) {
        return newNode;
      }

      const shouldReplace = [
        ts.isStringLiteral,
        ts.isArrowFunction,
        ts.isPropertyAccessExpression,
        ts.isCallExpression,
      ].some((typeAssertion) => typeAssertion(propertyAssignment.initializer));

      if (shouldReplace) {
        return propertyAssignment.initializer;
      }
    }

    return newNode;
  };

  return ts.visitNode(rootNode, visit);
};

const transformationResult = ts.transform(sourceFiles, [replaceVersions]);
const printer = ts.createPrinter({ newLine: ts.NewLineKind.LineFeed });

for (const transformed of transformationResult.transformed) {
  let output = printer.printNode(ts.EmitHint.Unspecified, transformed, transformed.getSourceFile());
  output = output.replace(/export const versioned/g, 'export const '); // remove versioned prefixs
  output = output.replace(/..\/generated\//g, './'); // adjust import paths
  output = output.replace(/: VersionedSelectorGroup/, ''); // remove typings for versioned selectors
  output = output.replace(/^;\n/gm, ''); // ts.factory.createEmptyStatement() leaves semicolon. need to investigate why
  const fileName = transformed.getSourceFile().fileName.replace(/\.ts$/, '.gen.ts');
  writeFile(resolve(join(process.cwd(), destDir, fileName)), output);
}
