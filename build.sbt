enablePlugins(ScalaJSPlugin)

name := "app"
scalaVersion := "2.13.7"

scalaJSUseMainModuleInitializer := true
mainClass := Some("app.Main")

libraryDependencies += "org.scala-js" %%% "scalajs-dom" % "2.1.0"
