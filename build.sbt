enablePlugins(ScalaJSPlugin, EsbuildPlugin)

name := "nostr-army-knife"
scalaVersion := "3.2.2"

scalaJSUseMainModuleInitializer := true

libraryDependencies += "com.armanbilge" %%% "calico" % "0.2.0-RC2"
libraryDependencies += "com.fiatjaf" %%% "snow" % "0.0.1-SNAPSHOT"
