enablePlugins(ScalaJSPlugin)
// enablePlugins(ScalaJSBundlerPlugin)

name := "app"
scalaVersion := "2.13.7"

scalaJSUseMainModuleInitializer := true

libraryDependencies += "org.scala-js" %%% "scalajs-dom" % "2.1.0"
libraryDependencies += "me.shadaj" %%% "slinky-core" % "0.7.0"
libraryDependencies += "me.shadaj" %%% "slinky-web" % "0.7.0"
